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

// GithubConfig is a github plugin config
type GithubConfig struct {
	// Token is an access token
	Token force.StringVar
}

type evaluatedConfig struct {
	token string
}

// CheckAndSetDefaults checks and sets default values
func (cfg *GithubConfig) CheckAndSetDefaults(ctx force.ExecutionContext) (*evaluatedConfig, error) {
	e := evaluatedConfig{}
	var err error
	if e.token, err = force.EvalString(ctx, cfg.Token); err != nil {
		return nil, trace.Wrap(err)
	}
	if e.token == "" {
		return nil, trace.BadParameter("set GithubConfig{Token: ``} parameter")
	}
	return &e, nil
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
		s.Branch = force.String(MasterBranch)
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
	cfg    evaluatedConfig
	client *GithubClient
}

// Github creates a new action setting up a github plugin
func Github(cfg GithubConfig) (force.Action, error) {
	return &NewPlugin{
		cfg: cfg,
	}, nil
}

// NewPlugin returns a function creating new plugins
type NewPlugin struct {
	cfg GithubConfig
}

// NewInstance returns a new instance
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Github
}

func (n *NewPlugin) Run(ctx force.ExecutionContext) error {
	ecfg, err := n.cfg.CheckAndSetDefaults(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	client, err := newGithubClient(ctx, *ecfg)
	if err != nil {
		return trace.Wrap(err)
	}
	p := &Plugin{cfg: *ecfg, client: client, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(GithubPlugin, p)
	return nil
}

// MarshalCode marshals plugin setup to code representation
func (n *NewPlugin) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(Github, n.cfg).MarshalCode(ctx)
}

// NewWatch finds the initialized github plugin and returns a new watch
type NewWatch struct {
}

// NewInstance returns a function creating new watchers
func (n *NewWatch) NewInstance(group force.Group) (force.Group, interface{}) {
	group.AddDefinition(KeyCommit, force.String(""))
	group.AddDefinition(KeyPR, force.Int(0))
	return group, func(src Source) (force.Channel, error) {
		pluginI, ok := group.GetPlugin(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).Watch(src)
	}
}

// NewPostStatusOf returns a function that wraps underlying action
// and tracks the result, posting the result back
type NewPostStatusOf struct {
}

// NewInstance returns a function creating new post status actions
func (n *NewPostStatusOf) NewInstance(group force.Group) (force.Group, interface{}) {
	// PostStatusOf creates a sequence, that's why it has to create a new lexical
	// scope (as sequence expects one to be created)
	scope := force.WithLexicalScope(group)
	return scope, func(inner ...force.Action) (force.Action, error) {
		pluginI, ok := group.GetPlugin(GithubPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).PostStatusOf(inner...)
	}
}

// NewPostStatus creates actions that posts new status
type NewPostStatus struct {
}

// NewInstance returns a function that creates new post status actions
func (n *NewPostStatus) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(status Status) (force.Action, error) {
		pluginI, ok := group.GetPlugin(GithubPlugin)
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

// MarshalCode marshals things to code
func (r *RepoWatcher) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(r.plugin.Watch, r.source).MarshalCode(ctx)
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
	// Those variables can be set, as they are defined by
	// PullRequests in a separate scope
	ctx.SetValue(force.ContextKey(KeyCommit), force.String(r.PR.LastCommit.OID))
	ctx.SetValue(force.ContextKey(KeyPR), force.Int(r.PR.Number))
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
