package git

import (
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
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

// Run sets up git plugin for the process group
func (n *Setup) Run(ctx force.ExecutionContext) error {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return trace.Wrap(err)
	}
	err := cfg.CheckAndSetDefaults()
	if err != nil {
		return trace.Wrap(err)
	}
	plugin := &Plugin{cfg: cfg, start: time.Now().UTC()}
	ctx.Process().Group().SetPlugin(Key, plugin)
	return nil
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

// Clone executes inner action and posts result of it's execution
// to github
func Clone(repo interface{}) (force.Action, error) {
	return &CloneAction{
		repo: repo,
	}, nil
}

// NewClone creates functions cloning repositories
type NewClone struct {
}

// NewInstance returns a function that wraps underlying action
// and tracks the result, posting the result back
func (n *NewClone) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, Clone
}

// CloneAction clones repository
type CloneAction struct {
	repo interface{}
}

func (p *CloneAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize Git plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	var repo Repo
	if err := force.EvalInto(ctx, p.repo, &repo); err != nil {
		return trace.Wrap(err)
	}

	log := force.Log(ctx)
	if repo.Into == "" {
		return trace.BadParameter("got empty Into variable")
	}
	fi, err := os.Stat(repo.Into)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	if !fi.IsDir() {
		return trace.BadParameter("Into variable is not an existing directory")
	}

	log.Infof("Cloning repository %v into %v.", repo.URL, repo.Into)
	start := time.Now()

	auth, err := plugin.cfg.Auth()
	if err != nil {
		return trace.Wrap(err)
	}

	r, err := git.PlainClone(repo.Into, false, &git.CloneOptions{
		Auth: auth,
		URL:  string(repo.URL),
	})
	if err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Cloned %v into %v in %v.", repo.URL, repo.Into, time.Now().Sub(start))

	if repo.Hash != "" {
		w, err := r.Worktree()
		if err != nil {
			return trace.Wrap(err)
		}

		err = w.Checkout(&git.CheckoutOptions{
			Hash: plumbing.NewHash(repo.Hash),
		})
		if err != nil {
			return trace.Wrap(err)
		}

		log.Infof("Checked out repository %v commit %v.", repo.URL, repo.Hash)
	}

	for i, subName := range repo.Submodules {
		if subName == "" {
			return trace.BadParameter("got empty submodule name at %v", i)
		}
		w, err := r.Worktree()
		if err != nil {
			return trace.Wrap(err)
		}
		log.Infof("Updating submodule %v.", subName)
		sub, err := w.Submodule(subName)
		if err != nil {
			return trace.Wrap(err)
		}
		err = sub.UpdateContext(ctx, &git.SubmoduleUpdateOptions{
			Init: true,
			Auth: auth,
		})
		if err != nil {
			return trace.Wrap(err)
		}
	}

	return nil
}

// MarshalCode marshals action into code representation
func (c *CloneAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Clone,
		Args:    []interface{}{c.repo},
	}
	return call.MarshalCode(ctx)
}
