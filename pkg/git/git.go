package git

import (
	"io/ioutil"
	"reflect"
	"strings"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Repo{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(force.FunctionName(Clone), &NewClone{})
//	scope.AddDefinition(force.FunctionName(Push), &NewPush{})
	scope.AddDefinition(KeySetup, &Setup{})
	return scope, nil
}

// Namespace is a wrapper around string to namespace a variable
type Namespace string

const (
	// Key is a name of the github plugin variable
	Key      = Namespace("git")
	KeySetup = "Setup"
)

// Config holds git configuration
type Config struct {
	// Token is an access token
	Token string
	// TokenFile is a path to access token
	TokenFile string
	// User force.StringVar
	User string
	// PrivateKeyFile is a path to SSH private key
	PrivateKeyFile string
	// KnownHostsFile is a file with known_hosts public keys
	KnownHostsFile string
}

func (cfg *Config) CheckAndSetDefaults() error {
	if cfg.TokenFile != "" {
		data, err := ioutil.ReadFile(cfg.TokenFile)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		cfg.Token = strings.TrimSpace(string(data))
	}
	if cfg.Token == "" && cfg.PrivateKeyFile == "" {
		return trace.BadParameter("set git.Config{Token:``} or git.Config{PrivateKey: ``} parameters")
	}
	if cfg.PrivateKeyFile != "" && cfg.User == "" {
		cfg.User = "git"
	}
	return nil
}

func (cfg *Config) Auth() (transport.AuthMethod, error) {
	if cfg.PrivateKeyFile != "" {
		keys, err := ssh.NewPublicKeysFromFile(cfg.User, cfg.PrivateKeyFile, "")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if cfg.KnownHostsFile != "" {
			helper, err := ssh.NewKnownHostsCallback(cfg.KnownHostsFile)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			keys.HostKeyCallback = helper
		}
		return keys, nil
	}
	return &http.BasicAuth{
		// this can be anything except an empty string
		Username: "token",
		Password: string(cfg.Token),
	}, nil
}

// Repo is a repository to clone
type Repo struct {
	URL string
	// Into into dir
	Into string
	// Hash is a commit hash to clone
	Hash string
	// Tag is a git tag to clone
	Tag string
	// Branch is a branch to clone
	Branch string
	// Submodules is an optional submodule to init
	Submodules []string
}

func (r *Repo) CheckAndSetDefaults() error {
	if r.URL == "" {
		return trace.BadParameter("set git.Repo{URL: ``} parameter")
	}
	if r.Into == "" {
		return trace.BadParameter("set git.Repo{Into: ``} parameter")
	}
	found := 0
	if r.Hash != "" {
		found++
	}
	if r.Tag != "" {
		found++
	}
	if r.Branch != "" {
		found++
	}
	if found == 0 {
		return trace.BadParameter("specify one of git.Repo{Tag: ``, Hash: ``, Branch: ``}")
	}
	if found > 1 {
		return trace.BadParameter("specify only one of git.Repo{Tag: ``, Hash: ``, Branch: ``}, not several at once")
	}
	return nil
}

// Plugin is a new plugin
type Plugin struct {
	// start is a plugin start time
	start time.Time
	cfg   Config
}

// Setup creates new plugins
type Setup struct {
	cfg interface{}
}

// NewInstance returns function creating new client bound to the process group
// and registers plugin variable
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) (force.Action, error) {
		return &Setup{
			cfg: cfg,
		}, nil
	}
}

func (n *Setup) Type() interface{} {
	return true
}

// Run sets up git plugin for the process group
func (n *Setup) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return false, trace.Wrap(err)
	}
	err := cfg.CheckAndSetDefaults()
	if err != nil {
		return false, trace.Wrap(err)
	}
	plugin := &Plugin{cfg: cfg, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(Key, plugin)
	return true, nil
}

// MarshalCode marshals plugin code to representation
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

