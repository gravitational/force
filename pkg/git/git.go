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
const GitPlugin = Key("Git")

// GitConfig is a configuration
type GitConfig struct {
	// Token is an access token
	Token force.StringVar
}

func (cfg *GitConfig) CheckAndSetDefaults(ctx force.ExecutionContext) (*evaluatedConfig, error) {
	ecfg := evaluatedConfig{}
	var err error
	if ecfg.token, err = force.EvalString(ctx, cfg.Token); err != nil {
		return nil, trace.Wrap(err)
	}
	if ecfg.token == "" {
		return nil, trace.BadParameter("set GitConfig{Token:``} parameter")
	}
	return &ecfg, nil
}

type evaluatedConfig struct {
	token string
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
	cfg   evaluatedConfig
}

// NewPlugin creates new plugins
type NewPlugin struct {
	cfg GitConfig
}

// NewInstance returns function creating new client bound to the process group
// and registers plugin variable
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg GitConfig) (force.Action, error) {
		return &NewPlugin{
			cfg: cfg,
		}, nil
	}
}

// Run sets up git plugin for the process group
func (n *NewPlugin) Run(ctx force.ExecutionContext) error {
	ecfg, err := n.cfg.CheckAndSetDefaults(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	plugin := &Plugin{cfg: *ecfg, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(GitPlugin, plugin)
	return nil
}

// MarshalCode marshals plugin code to representation
func (n *NewPlugin) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		FnName: string(GitPlugin),
		Args:   []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// Clone executes inner action and posts result of it's execution
// to github
func Clone(repo Repo) (force.Action, error) {
	if err := repo.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &CloneAction{
		repo: repo,
	}, nil
}

// NewClone creates functions cloning repositories
type NewClone struct {
}

// NewInstance returns a function that wraps underlying action
// and tracks the result, posting the result back
func (n *NewClone) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Clone
}

// CloneAction clones repository
type CloneAction struct {
	repo Repo
}

func (p *CloneAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(GitPlugin)
	if !ok {
		return trace.NotFound("initialize Git plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

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
			Password: string(plugin.cfg.token),
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

// MarshalCode marshals action into code representation
func (c *CloneAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Fn:   Clone,
		Args: []interface{}{c.repo},
	}
	return call.MarshalCode(ctx)
}
