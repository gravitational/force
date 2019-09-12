package kube

import (
	"reflect"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Job{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(KeySetup, &Setup{})
	scope.AddDefinition(KeyRun, &NewRun{})
	return scope, nil
}

// Namespace is a wrapper around string to namespace a variable
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key      = Namespace("kube")
	KeySetup = "Setup"
	KeyRun   = "Run"
)

// Config specifies kube plugin configuration
type Config struct {
	// Path is a path to kubernetes config file
	Path string
}

// CheckAndSetDefaults checks and sets defaults
func (cfg *Config) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	return nil
}

// Setup creates new plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns a new kubernetes client bound to the process group
// and registers plugin within variable
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) force.Action {
		return &Setup{cfg: cfg}
	}
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

func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	if err := cfg.CheckAndSetDefaults(ctx); err != nil {
		return trace.Wrap(err)
	}
	client, config, err := GetClient(cfg.Path)
	if err != nil {
		return trace.Wrap(err)
	}
	plugin := &Plugin{
		cfg:    cfg,
		client: client,
		config: config,
	}
	ctx.Process().Group().SetPlugin(Key, plugin)
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	cfg    Config
	client *kubernetes.Clientset
	config *rest.Config
}
