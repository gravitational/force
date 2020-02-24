package git

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	gitconfig "gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// NewPull specifies a new pull Action
type NewPull struct {
}

// NewInstance returns a function that pulls files to a git repository
func (n *NewPull) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(opts interface{}) (force.Action, error) {
		return &PullAction{
			options: opts,
		}, nil
	}
}

// PullOptions specify details about the pull
type PullOptions struct {
	// Directory is the path to the local git repostiory directory
	Directory string
	// RemoteName is the name of a remote repository
	RemoteName string
	// RemoteURL is the URL of a remote repository
	RemoteURL string
	// RefSpec is the reference to pull
	RefSpec string
}

// CheckAndSetDefaults checks and sets default values
func (o *PullOptions) CheckAndSetDefaults() error {
	if o.Directory == "" {
		o.Directory = "." // Current directory
	}

	return nil
}

// PullAction returns new pull actions
type PullAction struct {
	options interface{}
}

func (p *PullAction) Type() interface{} {
	return ""
}

// MarshalCode marshals the action into code representation
func (c *PullAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      KeyPull,
		Args:    []interface{}{c.options},
	}
	return call.MarshalCode(ctx)
}

// Eval pulls files to the git repository
func (p *PullAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var opts PullOptions
	if err := force.EvalInto(ctx, p.options, &opts); err != nil {
		return nil, trace.Wrap(err)
	}

	if err := opts.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}

	repo, err := git.PlainOpen(opts.Directory)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.NotFound("git plugin is not initialized")
	}

	plugin, ok := pluginI.(*Plugin)
	if !ok {
		return nil, trace.BadParameter("plugin is of unexpected type %T", pluginI)
	}

	auth, err := plugin.cfg.Auth()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	config := &git.PullOptions{
		Auth: auth,
		ReferenceName: plumbing.ReferenceName(opts.RefSpec),
	}

	if opts.RemoteName != "" {
		config.RemoteName = opts.RemoteName
	}

	if opts.RemoteURL != "" {
		_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
			Name: opts.RemoteName,
			URLs: []string{opts.RemoteURL},
		})
		if err != nil && err != git.ErrRemoteExists {
			return nil, trace.Wrap(err)
		}
	}

	return nil, trace.Wrap(w.Pull(config))
}
