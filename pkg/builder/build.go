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

// NewPlugin specifies builder plugins
type NewPlugin struct {
}

// NewInstance returns function creating new plugins based on configs
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg Config) (*Builder, error) {
		cfg.Context = group.Context()
		cfg.Group = group
		if err := cfg.CheckAndSetDefaults(); err != nil {
			return nil, trace.Wrap(err)
		}
		builder, err := New(cfg)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		group.SetPlugin(Plugin, builder)
		return builder, nil
	}
}

// NewBuild creates build actions
type NewBuild struct {
}

// NewInstance creates functions creating new Build action
func (n *NewBuild) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(img Image) (force.Action, error) {
		pluginI, ok := group.GetPlugin(Plugin)
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
			group.SetPlugin(Plugin, builder)
			return builder.NewBuild(img)
		}
		return pluginI.(*Builder).NewBuild(img)
	}
}

// NewBuild returns new action that builds image based on the spec
func (b *Builder) NewBuild(img Image) (force.Action, error) {
	return &BuildAction{
		Image:   img,
		Builder: b,
	}, nil
}

// BuildAction specifies biuldkit driven docker builds
type BuildAction struct {
	Builder *Builder
	Image   Image
}

func (b *BuildAction) Run(ctx force.ExecutionContext) error {
	return b.Builder.Run(ctx, b.Image)
}

func (b *BuildAction) String() string {
	return fmt.Sprintf("Build(tag=%v)", b.Image.Tag)
}
