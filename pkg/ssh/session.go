package ssh

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

type Action interface {
	BindClient(client *Client, env []Env) (Action, error)
	force.Action
}

type Env struct {
	Key string
	Val string
}

// Hosts enumerates hosts and helps to set environment
type Hosts struct {
	// Hosts is a list of hosts to target
	Hosts []string
	Env   []Env
	// ProxyJump is a proxy jump address (similar to ssh -J)
	ProxyJump string
}

// NewSession
type NewSession struct {
}

// NewInstance returns a new instance of a function with a new lexical scope
func (n *NewSession) NewInstance(group force.Group) (force.Group, interface{}) {
	return force.WithLexicalScope(group), Session
}

// Session groups sequence of commands together,
// if one fails, the chain stop execution
func Session(hosts interface{}, actions ...Action) (force.Action, error) {
	if len(actions) == 0 {
		return nil, trace.BadParameter("provide at least one session action")
	}
	return &SessionAction{
		hosts:   hosts,
		actions: actions,
	}, nil
}

// SessionAction runs actions in a sequence,
// if the action fails, next actions are not run
type SessionAction struct {
	actions []Action
	hosts   interface{}
}

// MarshalCode marshals action into code representation
func (p *SessionAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Session,
		Args:    make([]interface{}, 0, len(p.actions)+1),
	}
	call.Args = append(call.Args, p.hosts)
	for i := range p.actions {
		call.Args = append(call.Args, p.actions[i])
	}
	return call.MarshalCode(ctx)
}

func (s *SessionAction) Type() interface{} {
	elementType := reflect.TypeOf(s.actions[len(s.actions)-1].Type())
	sliceType := reflect.SliceOf(elementType)
	return reflect.Zero(sliceType).Interface()
}

// Eval runs actions in sequence using the passed scope
func (s *SessionAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	var hosts Hosts
	if err := force.EvalInto(ctx, s.hosts, &hosts); err != nil {
		return nil, trace.Wrap(err)
	}
	if len(hosts.Hosts) == 0 {
		return nil, trace.BadParameter("ssh.Session needs at least one host")
	}
	actions := make([]force.Action, len(hosts.Hosts))
	for i, h := range hosts.Hosts {
		actions[i] = &HostSequence{
			host:      h,
			actions:   s.actions,
			env:       hosts.Env,
			proxyJump: hosts.ProxyJump,
		}
	}
	p, err := force.Parallel(actions...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return p.Eval(ctx)
}

// HostSequence executes a series of commands in a sequence
type HostSequence struct {
	host      string
	actions   []Action
	env       []Env
	proxyJump string
}

// Type returns type of the sequence
func (s *HostSequence) Type() interface{} {
	return s.actions[len(s.actions)-1].Type()
}

// Eval runs actions in sequence on a single host
func (s *HostSequence) Eval(ctx force.ExecutionContext) (interface{}, error) {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.NotFound("initialize ssh plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	if s.proxyJump == "" {
		s.proxyJump = plugin.cfg.ProxyJump
	}
	client, err := dial(ctx, s.host, s.proxyJump, *plugin.clientConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer client.Close()

	forceActions := make([]force.Action, len(s.actions))
	for i := range s.actions {
		action, err := s.actions[i].BindClient(client, s.env)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		forceActions[i] = action
	}
	seq, err := force.Sequence(forceActions...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return seq.Eval(ctx)
}

// MarshalCode marshals action into code representation
func (p *HostSequence) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      "HostSequence",
		Args:    make([]interface{}, len(p.actions)),
	}
	for i := range p.actions {
		call.Args[i] = append(call.Args, p.actions[i])
	}
	return call.MarshalCode(ctx)
}

// Client wraps ssh client and optional proxy client
type Client struct {
	client      *ssh.Client
	proxyClient *ssh.Client
	config      *ssh.ClientConfig
}

func (c *Client) Close() error {
	var errors []error
	if c.proxyClient != nil {
		errors = append(errors, c.proxyClient.Close())
	}
	if c.client != nil {
		errors = append(errors, c.client.Close())
	}
	return trace.NewAggregate(errors...)
}

func dial(ctx context.Context, host string, proxyJump string, config ssh.ClientConfig) (*Client, error) {
	username, host := parseHost(host)
	if username != "" {
		config.User = username
	}

	d := &Dialer{}
	if proxyJump == "" {
		clt, err := d.Dial("tcp", host, &config)
		if err != nil {
			return nil, trace.ConnectionProblem(err, fmt.Sprintf("could not connect to %v", host))
		}
		return &Client{client: clt, config: &config}, nil
	}
	proxyClient, err := d.Dial("tcp", proxyJump, &config)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	proxyConn, err := proxyClient.Dial("tcp", host)
	if err != nil {
		defer proxyClient.Close()
		return nil, trace.ConnectionProblem(err, "failed connecting to node %v. %s", host, err)
	}

	conn, chans, _, err := newClientConn(ctx, proxyConn, host, &config)
	if err != nil {
		if strings.Contains(trace.Unwrap(err).Error(), "ssh: handshake failed") {
			proxyConn.Close()
			return nil, trace.AccessDenied(`access denied to %v connecting to %v`, config.User, host)
		}
		return nil, trace.Wrap(err)
	}

	// We pass an empty channel which we close right away to ssh.NewClient
	// because the client need to handle requests itself.
	emptyCh := make(chan *ssh.Request)
	close(emptyCh)

	return &Client{
		proxyClient: proxyClient,
		client:      ssh.NewClient(conn, chans, emptyCh),
		config:      &config,
	}, nil
}
