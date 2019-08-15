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
	Token force.String
}

func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.Token == "" {
		return trace.BadParameter("set Config{Token:``} parameter")
	}
	return nil
}

// Repo is a repository to clone
type Repo struct {
	URL force.String
	// Into into dir
	Into force.StringVar
	// Hash is a commit hash to clone
	Hash force.StringVar
	// Submodules is an optional submodule to init
	Submodules []force.StringVar
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

// NewPlugin creates new plugins
type NewPlugin struct {
}

// NewInstance returns function creating new client bound to the process group
// and registers plugin variable
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg Config) (*Plugin, error) {
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		p := &Plugin{Config: cfg, start: time.Now().UTC()}
		group.SetPlugin(GitPlugin, p)
		return p, nil
	}
}

// NewClone creates functions cloning repositories
type NewClone struct {
}

// NewInstance returns a function that wraps underlying action
// and tracks the result, posting the result back
func (n *NewClone) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(repo Repo) (force.Action, error) {
		pluginI, ok := group.GetPlugin(GitPlugin)
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

	into, err := p.repo.Into.Eval(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
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

	log.Infof("Cloning repository %v into %v.", p.repo.URL, into)
	start := time.Now()

	r, err := git.PlainClone(into, false, &git.CloneOptions{
		// The intended use of a GitHub personal access token is in replace of your password
		// because access tokens can easily be revoked.
		// https://help.github.com/articles/creating-a-personal-access-token-for-the-command-line/
		Auth: &http.BasicAuth{
			Username: "token", // this can be anything except an empty string
			Password: string(p.plugin.Token),
		},
		URL: string(p.repo.URL),
	})
	if err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Cloned %v into %v in %v.", p.repo.URL, into, time.Now().Sub(start))

	if p.repo.Hash != nil {
		hash, err := p.repo.Hash.Eval(ctx)
		if err != nil {
			return trace.Wrap(err)
		}
		if hash != "" {
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

	for i, subVar := range p.repo.Submodules {
		subName, err := subVar.Eval(ctx)
		if err != nil {
			return trace.Wrap(err)
		}
		if subName == "" {
			return trace.BadParameter("got empty submodule name at %v", i)
		}
		w, err := r.Worktree()
		if err != nil {
			return trace.Wrap(err)
		}
		log.Infof("Updating submodule %v.", subName)
		sub, err := w.Submodule(subName)
		if err != nil {
			return trace.Wrap(err)
		}
		err = sub.Update(&git.SubmoduleUpdateOptions{
			Init: true,
		})
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}
