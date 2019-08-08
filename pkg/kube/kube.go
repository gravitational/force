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

// Config is a configuration
type Config struct {
	// Path is a path to kubernetes config file
	Path force.String
}

// CheckAndSetDefaults checks and sets defaults
func (cfg *Config) CheckAndSetDefaults() error {
	return nil
}

// New returns a new instance of the kubernetes plugin
func New(cfg Config) (*Plugin, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	client, config, err := GetClient(string(cfg.Path))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &Plugin{
		Config: cfg,
		client: client,
		config: config,
	}, nil
}

// NewPluginFunc returns a new client bound to the process group
// and registers plugin within variable
func NewPluginFunc(group force.Group) func(cfg Config) (*Plugin, error) {
	return func(cfg Config) (*Plugin, error) {
		p, err := New(cfg)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		group.SetVar(KubePlugin, p)
		return p, nil
	}
}

// Plugin is a new plugin
type Plugin struct {
	Config
	client *kubernetes.Clientset
	config *rest.Config
}

// Run executes inner action and posts result of it's execution
// to github
func (p *Plugin) Run(job Job) (force.Action, error) {
	return &RunAction{
		job:    job,
		plugin: p,
	}, nil
}
