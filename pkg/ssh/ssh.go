package ssh

import (
	"io/ioutil"
	"net"
	"reflect"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

// Scope returns a new scope with all the functions and structs
// defined, this is the entrypoint into plugin as far as force is concerned
func Scope() (force.Group, error) {
	scope := force.WithLexicalScope(nil)
	err := force.ImportStructsIntoAST(scope,
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Hosts{}),
	)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	scope.AddDefinition(force.FunctionName(Command), &force.NopScope{Func: Command})
	scope.AddDefinition(force.FunctionName(Copy), &force.NopScope{Func: Copy})
	scope.AddDefinition(force.StructName(reflect.TypeOf(Setup{})), &Setup{})
	scope.AddDefinition(force.FunctionName(Local), &force.NopScope{Func: Local})
	scope.AddDefinition(force.FunctionName(Remote), &force.NopScope{Func: Remote})
	scope.AddDefinition(force.FunctionName(Session), &NewSession{})
	return scope, nil
}

//Namespace is a wrapper around string to namespace a variable in the context
type Namespace string

// Key is a name of the plugin variable
const Key = Namespace("ssh")

const (
	KeySetup  = "Setup"
	KeyConfig = "Config"
)

type KeyPair struct {
	// PrivateKeyFile is a required path to SSH private key
	PrivateKeyFile string
	// CertFile is an optional path to certificate
	CertFile string
}

func (k *KeyPair) Signer() (ssh.Signer, error) {
	if k.PrivateKeyFile == "" {
		return nil, trace.BadParameter("set ssh.KeyPair{PrivateKeyFile: ``} parameter")
	}
	privateKeyBytes, err := ioutil.ReadFile(k.PrivateKeyFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	signer, err := ssh.ParsePrivateKey(privateKeyBytes)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if k.CertFile == "" {
		return signer, nil
	}

	data, err := ioutil.ReadFile(k.CertFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	pubkey, _, _, _, err := ssh.ParseAuthorizedKey(data)
	if err != nil {
		return nil, trace.Wrap(err, "failed to parse SSH certificate")
	}

	cert, ok := pubkey.(*ssh.Certificate)
	if !ok {
		return nil, trace.BadParameter(
			"expected SSH certificate, got public key, if using public keys, simply supply a private key without public key")
	}

	signer, err = ssh.NewCertSigner(cert, signer)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return signer, nil
}

// Config is an ssh client configuration
type Config struct {
	// User is a linux login to try
	User string
	// KnownHostsFile is a list of hosts or certificates
	KnownHostsFile string
	// KeyPairs is a list of key pairs
	KeyPairs []KeyPair
	// ProxyJump is a proxy jump address (similar to ssh -J)
	ProxyJump string
}

// CheckAndSetDefaults checks and sets default values
func (cfg *Config) CheckAndSetDefaults() (*ssh.ClientConfig, error) {
	if cfg.User == "" {
		return nil, trace.BadParameter("set ssh.Config{User: ``} parameter")
	}

	if len(cfg.KeyPairs) == 0 {
		return nil, trace.BadParameter("set at least one ssh.Config{KeyPairs: } parameter")
	}

	if cfg.KnownHostsFile == "" {
		return nil, trace.BadParameter("set ssh.Config{KnownHostsFile: ``} parameter")
	}

	clientConfig := ssh.ClientConfig{
		User:    cfg.User,
		Timeout: defaultDialTimeout,
	}

	var signers []ssh.Signer
	for _, keyPair := range cfg.KeyPairs {
		signer, err := keyPair.Signer()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		signers = append(signers, signer)
	}

	clientConfig.Auth = []ssh.AuthMethod{ssh.PublicKeys(signers...)}

	if cfg.KnownHostsFile != "" {
		data, err := ioutil.ReadFile(cfg.KnownHostsFile)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		knownHosts, err := parseKnownHosts(data)
		if err != nil {
			return nil, trace.Wrap(err)
		}

		certChecker := ssh.CertChecker{
			IsHostAuthority: func(key ssh.PublicKey, addr string) bool {
				return checkHostKey(knownHosts, key, addr)
			},
			HostKeyFallback: func(addr string, remote net.Addr, key ssh.PublicKey) error {
				if !checkHostKey(knownHosts, key, addr) {
					return trace.AccessDenied("no matching keys in KnownHosts file")
				}
				return nil
			},
		}
		clientConfig.HostKeyCallback = certChecker.CheckHostKey
	}

	return &clientConfig, nil
}

// Plugin is a new logging plugin
type Plugin struct {
	cfg          Config
	clientConfig *ssh.ClientConfig
}

// Setup creates new instances of plugins
type Setup struct {
	cfg interface{}
}

func (n *Setup) Type() interface{} {
	return true
}

// NewInstance returns a new instance of a plugin bound to group
func (n *Setup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(cfg interface{}) (force.Action, error) {
		return &Setup{
			cfg: cfg,
		}, nil
	}
}

// MarshalCode marshals plugin setup to code
func (n *Setup) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := force.FnCall{
		Package: string(Key),
		FnName:  KeySetup,
		Args:    []interface{}{n.cfg},
	}
	return call.MarshalCode(ctx)
}

// Eval sets up logging plugin for the instance group
func (n *Setup) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var cfg Config
	if err := force.EvalInto(ctx, n.cfg, &cfg); err != nil {
		return nil, trace.Wrap(err)
	}
	clientConfig, err := cfg.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	p := &Plugin{
		cfg:          cfg,
		clientConfig: clientConfig,
	}
	ctx.Process().Group().SetPlugin(Key, p)
	return true, nil
}
