package builder

import (
	"fmt"
	"reflect"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Image{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(KeySetup, &Setup{})
	scope.AddDefinition(KeyBuild, &NewBuild{})
	scope.AddDefinition(KeyPush, &NewPush{})
	scope.AddDefinition(KeyPrune, &NewPrune{})
	return scope, nil
}

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

// Setup specifies builder plugins
type Setup struct {
	group force.Group
	cfg   interface{}
}

// NewInstance returns function creating new plugins based on configs
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) force.Action {
		return &Setup{
			group: group,
			cfg:   cfg,
		}
	}
}

// Run sets up plugin for the given process group
func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	cfg.Context = ctx
	cfg.Group = n.group
	builder, err := New(cfg)
	if err != nil {
		return trace.Wrap(err)
	}
	cfg.Group.SetPlugin(Key, builder)
	return nil
}

// MarshalCode marshals plugin to code representation
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// NewBuild creates build actions
type NewBuild struct {
}

// NewInstance creates functions creating new Build action
func (n *NewBuild) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(img interface{}) (force.Action, error) {
		return &BuildAction{image: img}, nil
	}
}

// BuildAction specifies biuldkit driven docker builds
type BuildAction struct {
	image interface{}
}

// Run runs build process
func (b *BuildAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize Builder plugin in the setup section")
	}
	return pluginI.(*Builder).Run(ctx, b.image)
}

func (b *BuildAction) String() string {
	return fmt.Sprintf("Build(tag=%v)", b.image)
}

// MarshalCode marshals the action into code representation
func (b *BuildAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyBuild,
		Args:    []interface{}{b.image},
	}
	return call.MarshalCode(ctx)
}
