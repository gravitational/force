package github

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

// NewPullRequestWatch finds the initialized github plugin and returns a new watch
type NewPullRequestWatch struct {
}

// NewInstance returns a function creating new watchers
func (n *NewPullRequestWatch) NewInstance(group force.Group) (force.Group, interface{}) {
	group.AddDefinition(force.KeyEvent, PullRequestEvent{})
	return group, func(srci interface{}) (force.Channel, error) {
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use github.Setup to initialize it")
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
		return &PullRequestWatcher{
			plugin: pluginI.(*Plugin),
			source: src,
			// TODO(klizhentas): queues have to be configurable
			eventsC: make(chan force.Event, 1024),
		}, nil
	}
}

// PullRequestWatcher is a repository watcher
type PullRequestWatcher struct {
	plugin  *Plugin
	source  Source
	eventsC chan force.Event
}

// String returns user friendly representation of the watcher
func (r *PullRequestWatcher) String() string {
	return fmt.Sprintf("PullRequestWatcher(%v)", r.source.Repo)
}

// MarshalCode marshals things to code
func (r *PullRequestWatcher) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyWatchPullRequests,
		Args:    []interface{}{r.source},
	}
	return call.MarshalCode(ctx)
}

// Start starts watch on a repo
func (r *PullRequestWatcher) Start(pctx context.Context) error {
	if r.source.Approval.Required && len(r.source.Approval.Teams) == 0 {
		return trace.BadParameter("approval is required, but no teams has been set, use Strings(`example/team`) to add a team")
	}
	go r.pollRepo(pctx)
	return nil
}

func (r *PullRequestWatcher) pollRepo(ctx context.Context) {
	log := force.Log(ctx)
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
	cache := make(map[int]PullRequest)
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
			pulls, err := r.updatedPullRequests(ctx, afterDate, cache)
			if err != nil {
				log.WithError(err).Warningf("Pull request check failed")
				continue
			}
			if len(pulls) == 0 {
				continue
			}
			lastPull := pulls[len(pulls)-1]
			if lastPull.LastUpdated().After(afterDate) {
				afterDate = lastPull.LastUpdated()
			}
			for _, pr := range pulls {
				event, err := r.processPR(ctx, users, pr)
				if err != nil {
					if !trace.IsNotFound(err) {
						log.WithError(err).Warningf("Failed to process PR.")
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

func (r *PullRequestWatcher) checkTriggers(ctx context.Context, pr pullRequestUpdate, approvers map[string]bool) (bool, error) {
	log := force.Log(ctx)
	if pr.newComment {
		retestRe, err := r.source.Trigger.RetestRegexp()
		if err != nil {
			return false, trace.Wrap(err)
		}
		if retestRe.MatchString(pr.LastComment.Body) {
			if !r.source.Approval.Required {
				log.Debugf(
					"PR %v got new retest %v, triggering", pr.Number, pr.LastComment.Body,
				)
				return true, nil
			}
			if approvers[pr.LastComment.Author.Login] {
				log.Debugf(
					"PR %v got new retest %v from approver %v, triggering", pr.Number, pr.LastComment.Body, pr.LastComment.Author.Login,
				)
				return true, nil
			}
		}
		if r.source.Approval.Required && approvers[pr.LastComment.Author.Login] {
			re, err := r.source.Approval.Regexp()
			if err != nil {
				return false, trace.Wrap(err)
			}
			if re.MatchString(pr.LastComment.Body) {
				log.Debugf(
					"PR %v got new approval %v, triggering", pr.Number, pr.LastCommit.Message,
				)
				return true, nil
			}
		}
	}
	if pr.newCommit {
		skipRe, err := r.source.Trigger.SkipRegexp()
		if err != nil {
			return false, trace.Wrap(err)
		}

		if !skipRe.MatchString(pr.LastCommit.Message) {
			log.Debugf(
				"PR %v got new commit %v, triggering", pr.Number, pr.LastCommit.Message,
			)
			return true, nil
		}
	}
	return false, nil
}

func (r *PullRequestWatcher) processPR(ctx context.Context, approvers map[string]bool, pr pullRequestUpdate) (*PullRequestEvent, error) {
	log := force.Log(ctx)

	matched, err := r.checkTriggers(ctx, pr, approvers)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if !matched {
		return nil, trace.NotFound("no pull request triggers matched")
	}
	if r.source.Approval.Required {
		if !approvers[pr.LastCommit.Author.User.Login] {
			log.Infof("Last commit was made by user %v who is not on the approval list, checking for approval.", pr.LastCommit.Author.User.Login)
			comment, err := r.checkApproval(ctx, approvers, pr.PullRequest)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			log.Infof("Request has been approved by %v with comment %v", comment.Author.Login, comment.Body)
		} else {
			log.Infof("Last commit was made by user %v who is on approval list, letting it through.", pr.LastCommit.Author.User.Login)
		}
	}
	event := &PullRequestEvent{
		Commit:      force.String(pr.LastCommit.OID),
		PR:          force.Int(pr.Number),
		PullRequest: pr.PullRequest,
		created:     time.Now().UTC(),
		Source:      r.source,
	}
	return event, nil
}

func (r *PullRequestWatcher) checkApproval(ctx context.Context, approvers map[string]bool, pr PullRequest) (*CommentObject, error) {
	re, err := r.source.Approval.Regexp()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	repo, err := r.source.Repository()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	comments, err := r.plugin.client.GetPullRequestComments(ctx, *repo, pr.Number)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	for _, comment := range comments {
		if _, ok := approvers[comment.Author.Login]; !ok {
			continue
		}
		if re.MatchString(comment.Body) {
			return &comment, nil
		}
	}
	return nil, trace.NotFound("approval is not found")
}

func fetchTeamsMembers(ctx context.Context, client *GithubClient, teams []string) (map[string]bool, error) {
	seen := make(map[string]bool)
	for _, team := range teams {
		org, slug, err := parseRepository(team)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		users, err := client.GetTeamMembers(ctx, org, slug)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		for _, u := range users {
			if _, ok := seen[u.Login]; !ok {
				seen[u.Login] = true
			}
		}
	}
	return seen, nil
}

// updatedPullRequests returns all pull requests updated after given date
func (r *PullRequestWatcher) updatedPullRequests(ctx context.Context, afterDate time.Time, cache map[int]PullRequest) ([]pullRequestUpdate, error) {
	var updatedPulls []pullRequestUpdate

	repo, err := r.source.Repository()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	pulls, err := r.plugin.client.GetOpenPullRequests(ctx, *repo)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	log := force.Log(ctx)

	for i := range pulls {
		pr := pulls[i]
		re, err := r.source.BranchRegexp()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if !re.MatchString(pr.PullRequestObject.BaseRefName) {
			log.Debugf(
				"PR %v branch %v did not match %v", pr.Number, pr.PullRequestObject.BaseRefName,
			)
			continue
		}
		if !pr.LastUpdated().After(afterDate) {
			continue
		}
		prev, ok := cache[pr.Number]
		if !ok {
			updatedPull := pullRequestUpdate{
				PullRequest: pr,
				newCommit:   pr.LastCommit.CommittedDate.Time.After(afterDate),
				newComment:  pr.LastComment.ID != "",
			}
			if updatedPull.newCommit || updatedPull.newComment {
				updatedPulls = append(updatedPulls, updatedPull)
			}
		} else {
			updatedPull := pullRequestUpdate{
				PullRequest: pr,
			}
			if prev.LastCommit.OID != pr.LastCommit.OID {
				updatedPull.newCommit = true
			}
			if prev.LastComment.Body != pr.LastComment.Body {
				updatedPull.newComment = true
			}
			if updatedPull.newCommit || updatedPull.newComment {
				updatedPulls = append(updatedPulls, updatedPull)
			}
		}
		cache[pr.Number] = pr
	}

	// Sort the prs by date
	sort.Slice(updatedPulls, func(i, j int) bool {
		return updatedPulls[j].LastUpdated().After(updatedPulls[i].LastUpdated())
	})

	return updatedPulls, nil
}

// Events returns events stream on a repository
func (r *PullRequestWatcher) Events() <-chan force.Event {
	return r.eventsC
}

// Done returns channel closed when repository watcher is closed
func (r *PullRequestWatcher) Done() <-chan struct{} {
	return nil
}

type CommitGetter interface {
	// GetCommit returns commit associated with the event
	GetCommit() string
	// GetSource returns source associated with the event
	GetSource() Source
}

// PullRequestEvent is a pull request event
type PullRequestEvent struct {
	PR          force.Int
	Commit      force.String
	Source      Source
	PullRequest PullRequest
	created     time.Time
}

// Created returns a time when the event was originated
func (r *PullRequestEvent) Created() time.Time {
	return r.created
}

// GetSource returns source associated with the event
func (r *PullRequestEvent) GetSource() Source {
	return r.Source
}

// GetCommit returns commit associated with the event
func (r *PullRequestEvent) GetCommit() string {
	return r.PullRequest.LastCommit.OID
}

// AddMetadata adds metadata to the logger
// and the context, such as commit id and PR number
func (r *PullRequestEvent) AddMetadata(ctx force.ExecutionContext) {
	logger := force.Log(ctx)
	logger = logger.AddFields(map[string]interface{}{
		KeyCommit: r.PullRequest.LastCommit.OID[:9],
		KeyPR:     r.PullRequest.Number,
	})
	force.SetLog(ctx, logger)
	// Those variables can be set, as they are defined by
	// PullRequests in a separate scope
	ctx.SetValue(force.ContextKey(force.KeyEvent), *r)
}

func (r *PullRequestEvent) String() string {
	return fmt.Sprintf("github pr %v, commit %v, updated %v with comment %q by %v",
		r.PullRequest.Number, r.PullRequest.LastCommit.OID[:9], r.PullRequest.LastUpdated().Format(force.HumanDateFormat),
		r.PullRequest.LastComment.Body, r.PullRequest.LastComment.Author.Login)
}
