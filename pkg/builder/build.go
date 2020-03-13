package builder

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// Namespace is a wrapper around string
// to namespace a variable
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key      = Namespace("builder")
	KeySetup = "Setup"
	KeyBuild = "Build"
	KeyPush  = "Push"
	KeyPrune = "Prune"
)

// Setup returns a function that sets up build plugin
func Setup(cfg Config) force.SetupFunc {
	return func(group force.Group) error {
		cfg.Group = group
		builder, err := New(cfg)
		if err != nil {
			return trace.Wrap(err)
		}
		group.SetPlugin(Key, builder)
		return nil
	}
}

// Build builds docker image
func Build(ctx force.ExecutionContext, img Image) error {
	if err := img.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize Builder plugin in the setup section")
	}
	_, err := pluginI.(*Builder).Eval(ctx, img)
	return err
}
