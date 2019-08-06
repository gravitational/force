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

// Key is a wrapper around string
// to namespace a variable
type Key string

// GithubPlugin is a name of the github plugin variable
const GithubPlugin = Key("github")

// Config is a github plugin config
type Config struct {
	// Token is an access token
	Token force.String
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Token == "" {
		return trace.BadParameter("set Config.Token parameter")
	}
	return nil
}

// Source is a source repository to watch
type Source struct {
	// Repo is a repository name to watch
	Repo force.String
	// Branch is a branch to watch PRs against
	Branch force.String
}

// CheckAndSetDefaults checks and sets default values
func (s *Source) CheckAndSetDefaults() error {
	if s.Repo == "" {
		return trace.BadParameter("provide Source{Repo: ``} parameter")
	}
	if _, err := s.Repository(); err != nil {
		return trace.Wrap(err)
	}
	if s.Branch == "" {
		s.Branch = MasterBranch
	}
	return nil
}

// Repository returns repository address
func (s *Source) Repository() (*Repository, error) {
	owner, repo, err := parseRepository(string(s.Repo))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Repository{Owner: owner, Name: repo}, nil
}

// WatchSource is a watch source
type WatchSource struct {
	// Repo is a repository name
	Repo Repository
	// Branch is a branch to watch
	Branch string
}

// Repository is a github repository
type Repository struct {
	// Owner is a repository owner
	Owner string
	// Name is a repository name
	Name string
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
func NewWatch(group force.Group) func(Source) (force.Channel, error) {
	return func(src Source) (force.Channel, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).Watch(src)
	}
}

// NewPostStatusOf returns a function that wraps underlying action
// and tracks the result, posting the result back
func NewPostStatusOf(group force.Group) func(...force.Action) (force.Action, error) {
	return func(inner ...force.Action) (force.Action, error) {
		pluginI, ok := group.GetVar(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).PostStatusOf(inner...)
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

// Watch returns a github source
func (g *Plugin) Watch(src Source) (force.Channel, error) {
	if err := src.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	repo, err := src.Repository()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &RepoWatcher{
		plugin: g,
		source: WatchSource{Repo: *repo, Branch: string(src.Branch)},
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan force.Event, 1024),
	}, nil
}

// RepoWatcher is a repository watcher
type RepoWatcher struct {
	plugin  *Plugin
	source  WatchSource
	eventsC chan force.Event
}

// String returns user friendly representation of the watcher
func (r *RepoWatcher) String() string {
	return fmt.Sprintf("RepoWatcher(%v/%v)", r.source.Repo.Owner, r.source.Repo.Name)
}

// Start starts watch on a repo
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
				event := &RepoEvent{PR: pr, created: time.Now().UTC(), Source: r.source}
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

	pulls, err := r.plugin.client.GetOpenPullRequests(r.source.Repo)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	for _, pr := range pulls {
		if pr.PullRequestObject.BaseRefName != r.source.Branch {
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

// Events returns events stream on a repository
func (r *RepoWatcher) Events() <-chan force.Event {
	return r.eventsC
}

// Done returns channel closed when repository watcher is closed
func (r *RepoWatcher) Done() <-chan struct{} {
	return nil
}

// RepoEvent is a repository event
type RepoEvent struct {
	Source  WatchSource
	PR      PullRequest
	created time.Time
}

// Created returns a time when the event was originated
func (r *RepoEvent) Created() time.Time {
	return r.created
}

// AddMetadata adds metadata to the logger
// and the context, such as commit id and PR number
func (r *RepoEvent) AddMetadata(ctx force.ExecutionContext) {
	logger := force.Log(ctx)
	logger = logger.AddFields(log.Fields{
		KeyCommit: r.PR.LastCommit.OID[:9],
		KeyPR:     r.PR.Number,
	})
	force.SetLog(ctx, logger)
	ctx.SetValue(force.ContextKey(KeyCommit), r.PR.LastCommit.OID)
	ctx.SetValue(force.ContextKey(KeyPR), r.PR.Number)
}

func (r *RepoEvent) String() string {
	return fmt.Sprintf("github pr %v, commit %v, updated %v with comment %q by %v",
		r.PR.Number, r.PR.LastCommit.OID[:9], r.PR.LastUpdated().Format(force.HumanDateFormat),
		r.PR.LastComment.Body, r.PR.LastComment.Author.Login)
}

const (
	// KeyCommit is a commit used in logs
	KeyCommit = "commit"
	// KeyPR is a pull request key used in logs
	KeyPR = "pr"
)
