package git

import (
	"os"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

// Key is a wrapper around string to namespace a variable
type Key string

// GitPlugin is a name of the github plugin variable
const GitPlugin = Key("git")

// Config is a configuration
type Config struct {
	// Token is an access token
	Token string
}

func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Token == "" {
		return trace.BadParameter("set Config{Token:``} parameter")
	}
	return nil
}

// Repo is a repository to clone
type Repo struct {
	URL string
	// Into into dir
	Into force.StringVar
	// Hash is a commit hash to clone
	Hash force.StringVar
}

func (r *Repo) CheckAndSetDefaults() error {
	if r.URL == "" {
		return trace.BadParameter("set Repo{URL: ``} parameter")
	}
	if r.Into == nil {
		return trace.BadParameter("set Repo{Into: ``} parameter")
	}
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	// start is a plugin start time
	start time.Time
	Config
}

// NewPlugin returns a new client bound to the process group
// and registers plugin within variable
func NewPlugin(group force.Group) func(cfg Config) (*Plugin, error) {
	return func(cfg Config) (*Plugin, error) {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		p := &Plugin{Config: cfg, start: time.Now().UTC()}
		group.SetVar(GitPlugin, p)
		return p, nil
	}
}

// NewClone returns a function that wraps underlying action
// and tracks the result, posting the result back
func NewClone(group force.Group) func(Repo) (force.Action, error) {
	return func(repo Repo) (force.Action, error) {
		pluginI, ok := group.GetVar(GitPlugin)
		if !ok {
			return nil, trace.NotFound("github plugin is not initialized, use Github to initialize it")
		}
		return pluginI.(*Plugin).Clone(repo)
	}
}

// Clone executes inner action and posts result of it's execution
// to github
func (p *Plugin) Clone(repo Repo) (force.Action, error) {
	if err := repo.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &CloneAction{
		repo:   repo,
		plugin: p,
	}, nil
}

// CloneAction clones repository
type CloneAction struct {
	repo   Repo
	plugin *Plugin
}

func (p *CloneAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)

	into := p.repo.Into.Value(ctx)
	if into == "" {
		return trace.BadParameter("got empty Into variable")
	}
	fi, err := os.Stat(into)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	if !fi.IsDir() {
		return trace.BadParameter("Into variable is not an existing directory")
	}

	r, err := git.PlainClone(into, false, &git.CloneOptions{
		// The intended use of a GitHub personal access token is in replace of your password
		// because access tokens can easily be revoked.
		// https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/
		Auth: &http.BasicAuth{
			Username: "token", // this can be anything except an empty string
			Password: p.plugin.Token,
		},
		URL: p.repo.URL,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	log.Debugf("Cloned %v into %v.", p.repo.URL, into)

	if p.repo.Hash != nil {
		if hash := p.repo.Hash.Value(ctx); hash != "" {
			w, err := r.Worktree()
			if err != nil {
				return trace.Wrap(err)
			}

			err = w.Checkout(&git.CheckoutOptions{
				Hash: plumbing.NewHash(hash),
			})
			if err != nil {
				return trace.Wrap(err)
			}

			log.Infof("Checked out repository %v commit %v.", p.repo.URL, hash)
		}
	}

	return nil
}
