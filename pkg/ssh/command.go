package ssh

import (
	"fmt"
	"io"
	"strings"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

// Command runs SSH command on a remote server
func Command(args ...force.StringVar) (force.Action, error) {
	a := CommandAction{}
	switch {
	case len(args) == 1:
		a.command = args[0]
	case len(args) == 2:
		a.host = args[0]
		a.command = args[1]
	default:
		return nil, trace.BadParameter("%v is unsupported amount of arguments, use as ssh.Command(`host:port`, command) or in ssh.Session(hosts, ssh.Command(command))", len(args))
	}
	return &a, nil
}

type CommandAction struct {
	command      force.StringVar
	host         force.StringVar
	client       *ssh.Client
	clientConfig *ssh.ClientConfig
	env          []Env
}

func (s *CommandAction) BindClient(client *ssh.Client, config *ssh.ClientConfig, env []Env) (Action, error) {
	if s.client != nil {
		return nil, trace.AlreadyExists("client already set")
	}
	return &CommandAction{
		host:         s.host,
		command:      s.command,
		client:       client,
		clientConfig: config,
		env:          env,
	}, nil
}

// Eval evaluates variable and returns string
func (s *CommandAction) Eval(ctx force.ExecutionContext) (string, error) {
	out := force.NewSyncBuffer()
	err := s.run(ctx, out)
	return strings.TrimSpace(out.String()), err
}

func (s *CommandAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)
	writer := force.Writer(log)
	defer writer.Close()
	return s.run(ctx, writer)
}

func (s *CommandAction) run(ctx force.ExecutionContext, writer io.Writer) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize ssh plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	command, err := force.EvalString(ctx, s.command)
	if err != nil {
		return trace.Wrap(err)
	}

	var client *ssh.Client
	if s.host != nil {
		host, err := force.EvalString(ctx, s.host)
		if err != nil {
			return trace.Wrap(err)
		}

		client, _, err = dial(host, *plugin.clientConfig)
		if err != nil {
			return trace.Wrap(err)
		}
		defer client.Close()
	} else {
		if s.client == nil {
			return trace.BadParameter("ssh.Command does not have host, it has to be used within ssh.Session")
		}
		client = s.client
	}

	session, err := client.NewSession()
	if err != nil {
		return trace.ConnectionProblem(err, "could not start session")
	}
	defer session.Close()

	for _, env := range s.env {
		if err := session.Setenv(env.Key, env.Val); err != nil {
			return trace.ConnectionProblem(err, "setting environment variable failed, perhaps missing AcceptEnv directive on the target server?")
		}
	}

	session.Stdout = writer
	session.Stderr = writer
	err = session.Start(command)
	if err != nil {
		return trace.ConnectionProblem(err, "could not execute command %v", command)
	}
	return session.Wait()
}

func parseHost(host string) (string, string) {
	parts := strings.SplitN(host, "@", 2)
	if len(parts) != 2 {
		return "", host
	}
	return parts[0], parts[1]
}

// MarshalCode marshals the action into code representation
func (s *CommandAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Command,
		Args:    []interface{}{s.command},
	}
	return call.MarshalCode(ctx)
}

func (s *CommandAction) String() string {
	return fmt.Sprintf("Command()")
}
