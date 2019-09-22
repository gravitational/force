package github

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

// NewBranchWatch finds the initialized github plugin and returns a new commit watch
type NewBranchWatch struct {
}

// NewInstance returns a function creating new watchers
func (n *NewBranchWatch) NewInstance(group force.Group) (force.Group, interface{}) {
	group.AddDefinition(force.KeyEvent, BranchEvent{})
	return group, func(srci interface{}) (force.Channel, error) {
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use github.Setup to initialize it", pluginI)
		}
		var src Source
		if err := force.EvalInto(force.EmptyContext(), srci, &src); err != nil {
			return nil, trace.Wrap(err)
		}
		if err := src.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		_, err := src.Repository()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return &BranchWatcher{
			plugin: pluginI.(*Plugin),
			source: src,
			// TODO(klizhentas): queues have to be configurable
			eventsC: make(chan force.Event, 1024),
		}, nil
	}
}

// BranchWatcher is a repository watcher
type BranchWatcher struct {
	plugin  *Plugin
	source  Source
	eventsC chan force.Event
}

// String returns user friendly representation of the watcher
func (r *BranchWatcher) String() string {
	return fmt.Sprintf("BranchWatcher(%v)", r.source.Repo)
}

// MarshalCode marshals things to code
func (r *BranchWatcher) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyWatchBranches,
		Args:    []interface{}{r.source},
	}
	return call.MarshalCode(ctx)
}

// Start starts watch on a repo
func (r *BranchWatcher) Start(pctx context.Context) error {
	if r.source.Approval.Required && len(r.source.Approval.Teams) == 0 {
		return trace.BadParameter("approval is required, but no teams has been set, use Strings(`example/team`) to add a team")
	}
	go r.pollRepo(pctx)
	return nil
}

func (r *BranchWatcher) pollRepo(ctx context.Context) {
	log := force.Log(ctx)
	cache := make(map[string]Branch)
	var users map[string]bool
	var err error
	if r.source.Approval.Required {
		users, err = fetchTeamsMembers(ctx, r.plugin.client, r.source.Approval.Teams)
		if err != nil {
			log.WithError(err).Warningf("failed to fetch teams members, approval requests will not succeed.")
		} else {
			log.Infof("Got approval team members: %v", users)
		}
	}
	afterDate := r.plugin.start
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
			if r.source.Approval.Required {
				users, err = fetchTeamsMembers(ctx, r.plugin.client, r.source.Approval.Teams)
				if err != nil {
					log.WithError(err).Warningf("failed to fetch teams members, approval requests will not succeed.")
				} else {
					log.Infof("Got approval team members: %v.", users)
				}
			}
		case <-time.After(5 * time.Second):
			branches, err := r.updatedBranches(ctx, afterDate, cache)
			if err != nil {
				log.WithError(err).Warningf("Branch check failes")
				continue
			}
			if len(branches) == 0 {
				continue
			}
			lastCommit := branches[len(branches)-1]
			if lastCommit.CommittedDate.After(afterDate) {
				afterDate = lastCommit.CommittedDate.Time
			}
			for _, branch := range branches {
				event, err := r.processBranch(ctx, users, branch)
				if err != nil {
					if !trace.IsNotFound(err) {
						log.WithError(err).Warningf("Failed to process Branch.")
					}
					continue
				}
				select {
				case r.eventsC <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// updatedBranches returns updated branches
func (r *BranchWatcher) updatedBranches(ctx context.Context, afterDate time.Time, cache map[string]Branch) ([]Branch, error) {
	repo, err := r.source.Repository()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	branches, err := r.plugin.client.GetBranches(ctx, *repo, r.source.Path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	var updatedBranches []Branch
	for i := range branches {
		branch := branches[i]
		if !branch.CommittedDate.After(afterDate) {
			continue
		}
		prev, ok := cache[branch.Name]
		if !ok {
			cache[branch.Name] = branch
			updatedBranches = append(updatedBranches, branch)
		} else {
			cache[branch.Name] = branch
			if prev.OID == branch.OID {
				continue
			}
			updatedBranches = append(updatedBranches, branch)
		}
	}
	sort.Slice(updatedBranches, func(i, j int) bool {
		return updatedBranches[j].CommittedDate.After(updatedBranches[j].CommittedDate.Time)
	})
	return updatedBranches, nil
}

func (r *BranchWatcher) processBranch(ctx context.Context, approvers map[string]bool, branch Branch) (*BranchEvent, error) {
	log := force.Log(ctx)

	matched, err := r.checkTriggers(ctx, branch, approvers)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if !matched {
		return nil, trace.NotFound("no pull request triggers matched")
	}
	if r.source.Approval.Required {
		if !approvers[branch.Author.User.Login] {
			return nil, trace.NotFound("Last commit was made by user %v who is not on the approval list, skipping CI.", branch.Author.User.Login)
		}
		log.Infof("Last commit was made by user %v who is on approval list, letting it through.", branch.Author.User.Login)
	}
	event := &BranchEvent{
		Commit:  force.String(branch.OID),
		Branch:  force.String(branch.Name),
		branch:  branch,
		created: time.Now().UTC(),
		Source:  r.source,
	}
	return event, nil
}

func (r *BranchWatcher) checkTriggers(ctx context.Context, branch Branch, approvers map[string]bool) (bool, error) {
	log := force.Log(ctx)
	skipRe, err := r.source.Trigger.SkipRegexp()
	if err != nil {
		return false, trace.Wrap(err)
	}
	if !skipRe.MatchString(branch.Message) {
		log.Debugf(
			"PR %v got new commit %v, triggering", branch.Name, branch.Message,
		)
		return true, nil
	}
	return false, nil
}

// Events returns events stream on a repository
func (r *BranchWatcher) Events() <-chan force.Event {
	return r.eventsC
}

// Done returns channel closed when repository watcher is closed
func (r *BranchWatcher) Done() <-chan struct{} {
	return nil
}

// BranchEvent is a commit event
type BranchEvent struct {
	Commit  force.String
	Branch  force.String
	branch  Branch
	Source  Source
	created time.Time
}

// Created returns a time when the event was originated
func (r *BranchEvent) Created() time.Time {
	return r.created
}

// GetCommit returns commit associated with the event
func (r *BranchEvent) GetCommit() string {
	return string(r.Commit)
}

// GetSource returns source associated with the event
func (r *BranchEvent) GetSource() Source {
	return r.Source
}

// AddMetadata adds metadata to the logger
// and the context, such as commit id and PR number
func (r *BranchEvent) AddMetadata(ctx force.ExecutionContext) {
	logger := force.Log(ctx)
	logger = logger.AddFields(map[string]interface{}{
		KeyCommit: r.Commit[:9],
		KeyBranch: r.Branch,
	})
	force.SetLog(ctx, logger)
	// Those variables can be set, as they are defined by
	// PullRequests in a separate scope
	ctx.SetValue(force.ContextKey(force.KeyEvent), *r)
}

func (r *BranchEvent) String() string {
	return fmt.Sprintf("github branch %v, commit %v", r.Branch, r.Commit)
}
