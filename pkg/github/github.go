package github

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// GithubKey is a wrapper around string
// to namespace a variable
type GithubKey string

// GithubPlugin is a name of the github plugin variable
const GithubPlugin = GithubKey("github")

type Config struct {
	// Token is an access token
	Token string
	// Repo is a repository to bind to
	Repo string
	// Branch is a branch to watch PRs against
	Branch string
}

func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Token == "" {
		return trace.BadParameter("set Config.Token parameter")
	}
	if cfg.Repo == "" {
		return trace.BadParameter("provide Config.Branch parameter")
	}
	if cfg.Branch == "" {
		cfg.Branch = MasterBranch
	}
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	// start is a plugin start time
	start  time.Time
	Config Config

	client *GithubClient
}

// NewPlugin returns a new client bound to the process group
// and registers plugin within variable
func NewPlugin(group force.Group) func(cfg Config) (*Plugin, error) {
	return func(cfg Config) (*Plugin, error) {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		client, err := NewGithubClient(group.Context(), cfg)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		p := &Plugin{Config: cfg, client: client, start: time.Now().UTC()}
		group.SetVar(GithubPlugin, p)
		return p, nil
	}
}

// NewWatch finds the initialized github plugin and returns a new watch
func NewWatch(group force.Group) func() (force.Channel, error) {
	return func() (force.Channel, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).Watch()
	}
}

// NewPostStatus posts new status
func NewPostStatus(group force.Group) func(Status) (force.Action, error) {
	return func(status Status) (force.Action, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).PostStatus(status)
	}
}

// NewPostPending posts pending status
func NewPostPending(group force.Group) func() (force.Action, error) {
	return func() (force.Action, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).PostPending()
	}
}

// NewPostResult posts result
func NewPostResult(group force.Group) func() (force.Action, error) {
	return func() (force.Action, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).PostResult()
	}
}

// Github returns a github source
func (g *Plugin) Watch() (force.Channel, error) {
	return &RepoWatcher{
		plugin: g,
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan force.Event, 1024),
	}, nil
}

type RepoWatcher struct {
	plugin  *Plugin
	eventsC chan force.Event
}

func (r *RepoWatcher) String() string {
	return fmt.Sprintf("RepoWatcher(%v/%v)", r.plugin.client.Owner, r.plugin.client.Repository)
}

func (r *RepoWatcher) Start(pctx context.Context) error {
	go r.pollRepo(pctx)
	return nil
}

func (r *RepoWatcher) pollRepo(ctx context.Context) {
	afterDate := r.plugin.start
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			pulls, err := r.updatedPullRequests(afterDate)
			if err != nil {
				log.Warningf("Pull request check failed: %v", trace.DebugReport(err))
				continue
			}
			if len(pulls) == 0 {
				continue
			}
			afterDate = pulls[len(pulls)-1].LastUpdated()
			for _, pr := range pulls {
				event := &RepoEvent{PR: pr}
				select {
				case r.eventsC <- event:
					log.Debugf("-> %v", event)
				case <-ctx.Done():
					return
				}
			}
		}
	}
}

// updatedPullRequests returns all pull requests updated after given date
func (r *RepoWatcher) updatedPullRequests(afterDate time.Time) (PullRequests, error) {
	var updatedPulls PullRequests

	pulls, err := r.plugin.client.GetOpenPullRequests()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, pr := range pulls {
		if pr.PullRequestObject.BaseRefName != r.plugin.Config.Branch {
			continue
		}
		if !pr.LastUpdated().After(afterDate) {
			continue
		}
		updatedPulls = append(updatedPulls, pr)
	}

	// Sort the commits by date
	sort.Sort(updatedPulls)

	return updatedPulls, nil
}

func (r *RepoWatcher) Events() <-chan force.Event {
	return r.eventsC
}

func (r *RepoWatcher) Done() <-chan struct{} {
	return nil
}

type RepoEvent struct {
	PR PullRequest
}

func (r *RepoEvent) String() string {
	return fmt.Sprintf("RepoEvent(PR=%v, commit=%v, updated=%v, comment=%q by %v)",
		r.PR.Number, r.PR.LastCommit.OID, r.PR.LastUpdated(), r.PR.LastComment.Body, r.PR.LastComment.Author.Login)
}
