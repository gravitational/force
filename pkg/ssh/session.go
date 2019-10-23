package ssh

import (
	"fmt"
	"reflect"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

type Action interface {
	BindClient(client *ssh.Client, config *ssh.ClientConfig, env []Env) (Action, error)
	force.Action
}

type Env struct {
	Key string
	Val string
}

// Hosts enumerates hosts and helps to set environment
type Hosts struct {
	Hosts []string
	Env   []Env
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
		actions[i] = &HostSequence{host: h, actions: s.actions, env: hosts.Env}
	}
	p, err := force.Parallel(actions...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return p.Eval(ctx)
}

// HostSequence executes a series of commands in a sequence
type HostSequence struct {
	host    string
	actions []Action
	env     []Env
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

	client, config, err := dial(s.host, *plugin.clientConfig)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	defer client.Close()

	forceActions := make([]force.Action, len(s.actions))
	for i := range s.actions {
		action, err := s.actions[i].BindClient(client, config, s.env)
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

func dial(host string, config ssh.ClientConfig) (*ssh.Client, *ssh.ClientConfig, error) {
	username, host := parseHost(host)
	if username != "" {
		config.User = username
	}

	d := &Dialer{}
	client, err := d.Dial("tcp", host, &config)
	if err != nil {
		return nil, nil, trace.ConnectionProblem(err, fmt.Sprintf("could not connect to %v", host))
	}
	return client, &config, nil
}
