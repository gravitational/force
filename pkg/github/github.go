package github

import (
	"io/ioutil"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
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
	scope.AddDefinition(KeyWatchPullRequests, &NewPullRequestWatch{})
	scope.AddDefinition(KeyWatchBranches, &NewBranchWatch{})
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
	KeyWatchBranches     = "Branches"
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

// Approval configures approval flow
type Approval struct {
	Required bool
	Teams    []string
	Pattern  string
}

// Regexp returns approval regexp
func (a *Approval) Regexp() (*regexp.Regexp, error) {
	if a.Pattern == "" {
		a.Pattern = ".*ok to test.*"
	}
	re, err := regexp.Compile(a.Pattern)
	if err != nil {
		return nil, trace.BadParameter("failed to parse Pattern: %q, must be valid regular expression, e.g. `.*ok to test.*`", a.Pattern)
	}
	return re, nil
}

// Trigger sets up additional testing triggers
type Trigger struct {
	Disabled bool
	// RetestPattern triggers retest on a pattern
	RetestPattern string
	// SkipPattern skips pull request test
	SkipPattern string
}

// RetestRegexp returns retest regexp
func (a *Trigger) RetestRegexp() (*regexp.Regexp, error) {
	if a.RetestPattern == "" {
		a.RetestPattern = ".*retest this.*"
	}
	re, err := regexp.Compile(a.RetestPattern)
	if err != nil {
		return nil, trace.BadParameter("failed to parse RetestPattern: %q, must be valid regular expression, e.g. `.*ok to test.*`", a.RetestPattern)
	}
	return re, nil
}

// SkipRegexp returns skip match regexp
func (a *Trigger) SkipRegexp() (*regexp.Regexp, error) {
	if a.SkipPattern == "" {
		a.SkipPattern = ".*skip ci.*"
	}
	re, err := regexp.Compile(a.SkipPattern)
	if err != nil {
		return nil, trace.BadParameter("failed to parse SkipPattern: %q, must be valid regular expression, e.g. `.*skip ci.*`", a.SkipPattern)
	}
	return re, nil
}

// Source is a source repository to watch
type Source struct {
	// Repo is a repository name to watch
	Repo string
	// Branch is a branch to watch PRs against
	Branch string
	// Approval sets up approval process
	Approval Approval
	// Trigger configures trigger
	Trigger Trigger
	// Path filters out commits without changes matching the path (directory)
	Path string
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
	if _, err := s.Approval.Regexp(); err != nil {
		return trace.Wrap(err)
	}
	if _, err := s.Trigger.RetestRegexp(); err != nil {
		return trace.Wrap(err)
	}
	if _, err := s.Trigger.SkipRegexp(); err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Repository returns repository address
func (s Source) Repository() (*Repository, error) {
	owner, repo, err := parseRepository(s.Repo)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Repository{Owner: owner, Name: repo}, nil
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

func (n *Setup) Type() interface{} {
	return true
}

// NewInstance returns a new instance
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Github
}

func (n *Setup) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	client, err := newGithubClient(ctx, cfg)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	p := &Plugin{cfg: cfg, client: client, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(Key, p)
	return true, nil
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

const (
	// KeyCommit is a commit used in logs
	KeyCommit = "commit"
	// KeyBranch
	KeyBranch = "branch"
	// KeyPR is a pull request key used in logs
	KeyPR = "pr"
)
