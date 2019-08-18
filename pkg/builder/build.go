package builder

import (
	"fmt"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// Key is a wrapper around string
// to namespace a variable
type Key string

// Plugin is a name of the github plugin variable
const Plugin = Key("Builder")

// NewPlugin specifies builder plugins
type NewPlugin struct {
	cfg BuilderConfig
}

// NewInstance returns function creating new plugins based on configs
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg BuilderConfig) force.Action {
		cfg.Group = group
		return &NewPlugin{
			cfg: cfg,
		}
	}
}

// Run sets up plugin for the given process group
func (n *NewPlugin) Run(ctx force.ExecutionContext) error {
	n.cfg.Context = ctx
	builder, err := New(n.cfg)
	if err != nil {
		return trace.Wrap(err)
	}
	n.cfg.Group.SetPlugin(Plugin, builder)
	return nil
}

// MarshalCode marshals plugin to code representation
func (n *NewPlugin) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		FnName: string(Plugin),
		Args:   []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// Build creates build actions
func Build(img Image) (force.Action, error) {
	return &BuildAction{
		image: img,
	}, nil
}

// NewBuild creates build actions
type NewBuild struct {
}

// NewInstance creates functions creating new Build action
func (n *NewBuild) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Build
}

// BuildAction specifies biuldkit driven docker builds
type BuildAction struct {
	image Image
}

// Run runs build process
func (b *BuildAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Plugin)
	if !ok {
		return trace.NotFound("initialize Builder plugin in the setup section")
	}
	return pluginI.(*Builder).Run(ctx, b.image)
}

func (b *BuildAction) String() string {
	return fmt.Sprintf("Build(tag=%v)", b.image.Tag)
}

// MarshalCode marshals the action into code representation
func (b *BuildAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(Build, b.image).MarshalCode(ctx)
}
