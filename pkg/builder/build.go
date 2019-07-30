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
const Plugin = Key("builder")

// NewPlugin returns a new builder with some configuration
func NewPlugin(group force.Group) func(cfg Config) (*Builder, error) {
	return func(cfg Config) (*Builder, error) {
		cfg.Context = group.Context()
		cfg.Group = group
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		builder, err := New(cfg)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		group.SetVar(Plugin, builder)
		return builder, nil
	}
}

// NewBuild returns a new Build action
func NewBuild(group force.Group) func(Image) (force.Action, error) {
	return func(img Image) (force.Action, error) {
		pluginI, ok := group.GetVar(Plugin)
		if !ok {
			// plugin is not initialized, use defaults
			group.Logger().Debugf("Builder plugin is not initialized, using default.")
			builder, err := New(Config{
				Context: group.Context(),
				Group:   group,
			})
			if err != nil {
				return nil, trace.Wrap(err)
			}
			return builder.NewBuild(img)
		}
		return pluginI.(*Builder).NewBuild(img)
	}
}

// NewBuild returns new action that builds image based on the spec
func (b *Builder) NewBuild(img Image) (force.Action, error) {
	if err := img.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &BuildAction{
		Image:   img,
		Builder: b,
	}, nil
}

type BuildAction struct {
	Builder *Builder
	Image   Image
}

func (b *BuildAction) Run(ctx force.ExecutionContext) (force.ExecutionContext, error) {
	return ctx, b.Builder.Run(ctx, b.Image)
}

func (b *BuildAction) String() string {
	return fmt.Sprintf("Build(tag=%v)", b.Image.Tag)
}
