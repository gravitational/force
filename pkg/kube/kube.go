package kube

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Key is a wrapper around string to namespace a variable
type Key string

// KubePlugin is a name of the github plugin variable
const KubePlugin = Key("kube")

// kubeConfig is a configuration
type KubeConfig struct {
	// Path is a path to kubernetes config file
	Path force.StringVar
}

// CheckAndSetDefaults checks and sets defaults
func (cfg *KubeConfig) CheckAndSetDefaults(ctx force.ExecutionContext) (*evaluatedConfig, error) {
	ecfg := evaluatedConfig{}
	var err error
	if ecfg.path, err = force.EvalString(ctx, cfg.Path); err != nil {
		return nil, trace.Wrap(err)
	}
	return &ecfg, nil
}

type evaluatedConfig struct {
	path string
}

// Kube returns a new instance of the kubernetes plugin
func Kube(cfg KubeConfig) (*NewPlugin, error) {
	return &NewPlugin{cfg: cfg}, nil
}

// NewPlugin specifies new plugins
type NewPlugin struct {
	cfg KubeConfig
}

// NewInstance returns a new kubernetes client bound to the process group
// and registers plugin within variable
func (n *NewPlugin) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Kube
}

// MarshalCode marshals plugin to code representation
func (n *NewPlugin) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	return force.NewFnCall(Kube, n.cfg).MarshalCode(ctx)
}

func (n *NewPlugin) Run(ctx force.ExecutionContext) error {
	cfg, err := n.cfg.CheckAndSetDefaults(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	client, config, err := GetClient(cfg.path)
	if err != nil {
		return trace.Wrap(err)
	}
	plugin := &Plugin{
		cfg:    *cfg,
		client: client,
		config: config,
	}
	ctx.Process().Group().SetPlugin(KubePlugin, plugin)
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	cfg    evaluatedConfig
	client *kubernetes.Clientset
	config *rest.Config
}
