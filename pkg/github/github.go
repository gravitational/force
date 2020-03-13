package github

import (
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

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
	// BranchPattern is a branch regexp pattern to watch PRs against
	BranchPattern string
	// Approval sets up approval process
	Approval Approval
	// Trigger configures trigger
	Trigger Trigger
	// Path filters out commits without changes matching the path (directory)
	Path string
}

// BranchRegexp returns branch match regexp
func (s *Source) BranchRegexp() (*regexp.Regexp, error) {
	re, err := regexp.Compile(s.BranchPattern)
	if err != nil {
		return nil, trace.BadParameter("failed to parse BranchPattern: %q, must be valid regular expression, e.g. `.*`", s.BranchPattern)
	}
	return re, nil
}

// CheckAndSetDefaults checks and sets default values
func (s *Source) CheckAndSetDefaults() error {
	if s.Repo == "" {
		return trace.BadParameter("provide github.Source{Repo: ``} parameter")
	}
	if _, err := s.Repository(); err != nil {
		return trace.Wrap(err)
	}
	if s.BranchPattern == "" {
		s.BranchPattern = MasterBranch
	}
	if _, err := s.BranchRegexp(); err != nil {
		return trace.Wrap(err)
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

// Setup returns a function that sets up github plugin
func Setup(cfg Config) force.SetupFunc {
	return func(group force.Group) error {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
		client, err := newGithubClient(group.Context(), cfg)
		if err != nil {
			return trace.Wrap(err)
		}
		p := &Plugin{cfg: cfg, client: client, start: time.Now().UTC()}
		group.SetPlugin(Key, p)
		return nil
	}
}

const (
	// KeyCommit is a commit used in logs
	KeyCommit = "commit"
	// KeyBranch
	KeyBranch = "branch"
	// KeyPR is a pull request key used in logs
	KeyPR = "pr"
)
