package git

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	gitconfig "gopkg.in/src-d/go-git.v4/config"
)

// NewPush specifies a new push Action
type NewPush struct {
}

// NewPush returns a function that pushes updates to a git repository
func (n *NewPush) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(opts interface{}) (force.Action, error) {
		return &PushAction{
			options: opts,
		}, nil
	}
}

// PushOptions specify details about the push action
type PushOptions struct {
	// Directory is the path to the local git repostiory directory
	Directory string
	// RemoteName is the name of a remote repository
	RemoteName string
	// RemoteURL is the URL of a remote repository
	RemoteURL string
	// RefSpecs specifies which destination references are being updated
	RefSpecs []string
}

// CheckAndSetDefaults checks and sets default values
func (o *PushOptions) CheckAndSetDefaults() error {
	if o.Directory == "" {
		o.Directory = "." // Current directory
	}

	if o.RemoteURL != "" && o.RemoteName == "" {
		return trace.BadParameter("both the remote URL and the remote name must be specified")
	}

	return nil
}

// PushAction pushes to a repository
type PushAction struct {
	options interface{}
}

func (p *PushAction) Type() interface{} {
	return ""
}

// MarshalCode marshals action into code representation
func (c *PushAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      KeyPush,
		Args:    []interface{}{c.options},
	}
	return call.MarshalCode(ctx)
}

// Eval pushes the latest commits to a git repository
func (p *PushAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var opts PushOptions
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

	config := &git.PushOptions{
		Auth: auth,
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

	for _, refSpec := range opts.RefSpecs {
		config.RefSpecs = append(config.RefSpecs, gitconfig.RefSpec(refSpec))
	}

	return nil, trace.Wrap(repo.Push(config))
}

