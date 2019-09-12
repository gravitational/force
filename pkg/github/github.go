package github

import (
	"context"
	"fmt"
	"io/ioutil"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Source{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(KeySetup, &Setup{})
	scope.AddDefinition(KeyWatchPullRequests, &NewWatch{})
	scope.AddDefinition(KeyPostStatusOf, &NewPostStatusOf{})
	scope.AddDefinition(KeyPostStatus, &NewPostStatus{})
	return scope, nil
}

//Namespace is a wrapper around string to namespace a variable in the context
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key                  = Namespace("github")
	KeyWatchPullRequests = "PullRequests"
	KeySetup             = "Setup"
	KeyPostStatusOf      = "PostStatusOf"
	KeyPostStatus        = "PostStatusOf"
)

// Config is a github plugin config
type Config struct {
	// Token is an access token
	Token string
	// TokenFile is a path to access token
	TokenFile string
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.TokenFile != "" {
		data, err := ioutil.ReadFile(cfg.TokenFile)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		cfg.Token = strings.TrimSpace(string(data))
	}
	if cfg.Token == "" {
		return trace.BadParameter("set github.Config{Token: ``} parameter")
	}
	return nil
}

// Source is a source repository to watch
type Source struct {
	// Repo is a repository name to watch
	Repo string
	// Branch is a branch to watch PRs against
	Branch string
}

// CheckAndSetDefaults checks and sets default values
func (s *Source) CheckAndSetDefaults() error {
	if s.Repo == "" {
		return trace.BadParameter("provide github.Source{Repo: ``} parameter")
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
	owner, repo, err := parseRepository(s.Repo)
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
	cfg    Config
	client *GithubClient
}

// Github creates a new action setting up a github plugin
func Github(cfg interface{}) (force.Action, error) {
	return &Setup{
		cfg: cfg,
	}, nil
}

// Setup creates new instances of plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns a new instance
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Github
}

func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	client, err := newGithubClient(ctx, cfg)
	if err != nil {
		return trace.Wrap(err)
	}
	p := &Plugin{cfg: cfg, client: client, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(Key, p)
	return nil
}

// MarshalCode marshals plugin setup to code representation
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := force.FnCall{
		Package: string(Key),
		FnName:  "Setup",
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// NewWatch finds the initialized github plugin and returns a new watch
type NewWatch struct {
}

// NewInstance returns a function creating new watchers
func (n *NewWatch) NewInstance(group force.Group) (force.Group, interface{}) {
	group.AddDefinition(force.KeyEvent, RepoEvent{})
	return group, func(src interface{}) (force.Channel, error) {
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use github.Setup to initialize it")
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
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use github.Setup to initialize it")
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
		pluginI, ok := group.GetPlugin(Key)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use github.Setup to initialize it")
		}
		return pluginI.(*Plugin).PostStatus(status)
	}
}

// Watch returns a github source
func (g *Plugin) Watch(srci interface{}) (force.Channel, error) {
	var src Source
	if err := force.EvalInto(force.EmptyContext(), srci, &src); err != nil {
		return nil, trace.Wrap(err)
	}
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
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyWatchPullRequests,
		Args:    []interface{}{r.source},
	}
	return call.MarshalCode(ctx)
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
				event := &RepoEvent{
					Commit:      force.String(pr.LastCommit.OID),
					PR:          force.Int(pr.Number),
					PullRequest: pr,
					created:     time.Now().UTC(),
					Source:      r.source,
				}
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
	PR          force.Int
	Commit      force.String
	Source      WatchSource
	PullRequest PullRequest
	created     time.Time
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
		KeyCommit: r.PullRequest.LastCommit.OID[:9],
		KeyPR:     r.PullRequest.Number,
	})
	force.SetLog(ctx, logger)
	// Those variables can be set, as they are defined by
	// PullRequests in a separate scope
	ctx.SetValue(force.ContextKey(force.KeyEvent), *r)
}

func (r *RepoEvent) String() string {
	return fmt.Sprintf("github pr %v, commit %v, updated %v with comment %q by %v",
		r.PullRequest.Number, r.PullRequest.LastCommit.OID[:9], r.PullRequest.LastUpdated().Format(force.HumanDateFormat),
		r.PullRequest.LastComment.Body, r.PullRequest.LastComment.Author.Login)
}

const (
	// KeyCommit is a commit used in logs
	KeyCommit = "commit"
	// KeyPR is a pull request key used in logs
	KeyPR = "pr"
)
