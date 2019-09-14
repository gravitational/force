package ssh

import (
	"fmt"
	"net"

	"github.com/gravitational/force"

	"github.com/gravitational/force/pkg/ssh/scp"
	"github.com/gravitational/trace"
	"golang.org/x/crypto/ssh"
)

type Target struct {
	Local bool
	Path  force.StringVar
}

func Local(path force.StringVar) Target {
	return Target{
		Local: true,
		Path:  path,
	}
}

func Remote(path force.StringVar) Target {
	return Target{
		Local: false,
		Path:  path,
	}
}

// Copy runs SSH command on a remote server
func Copy(args ...interface{}) (force.Action, error) {
	c := CopyAction{}
	var ok bool
	switch len(args) {
	case 3:
		c.host, ok = args[0].(force.StringVar)
		if !ok {
			return nil, trace.BadParameter("expected first argument to be string")
		}
		c.source, ok = args[1].(Target)
		if !ok {
			return nil, trace.BadParameter("expected second argument to be ssh.Local or ssh.Remote")
		}
		c.destination, ok = args[2].(Target)
		if !ok {
			return nil, trace.BadParameter("expected third argument to be ssh.Local or ssh.Remote")
		}
		return &c, nil
	case 2:
		c.source, ok = args[0].(Target)
		if !ok {
			return nil, trace.BadParameter("expected first argument to be ssh.Local or ssh.Remote")
		}
		c.destination, ok = args[1].(Target)
		if !ok {
			return nil, trace.BadParameter("expected second argument to be ssh.Local or ssh.Remote")
		}
		return &c, nil
	}
	return nil, trace.BadParameter("%v is unsupported amount of arguments, use as ssh.Copy(`host:port`, source, dest) or in ssh.Session(hosts, ssh.Copy(source, dest))", len(args))
}

type CopyAction struct {
	host         force.StringVar
	source       Target
	destination  Target
	client       *ssh.Client
	clientConfig *ssh.ClientConfig
}

func (s *CopyAction) BindClient(client *ssh.Client, config *ssh.ClientConfig) (Action, error) {
	if s.client != nil {
		return nil, trace.AlreadyExists("client already set")
	}
	return &CopyAction{
		host:         s.host,
		source:       s.source,
		destination:  s.destination,
		client:       client,
		clientConfig: config,
	}, nil
}

func (s *CopyAction) Run(ctx force.ExecutionContext) error {
	log := force.Log(ctx)

	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize ssh plugin in the setup section")
	}
	plugin := pluginI.(*Plugin)

	var client *ssh.Client
	var clientConfig *ssh.ClientConfig
	if s.host != nil {
		host, err := force.EvalString(ctx, s.host)
		if err != nil {
			return trace.Wrap(err)
		}

		client, clientConfig, err = dial(host, *plugin.clientConfig)
		if err != nil {
			return trace.Wrap(err)
		}

		defer client.Close()
	} else {
		if s.client == nil {
			return trace.BadParameter("ssh.Command does not have host, it has to be used within ssh.Session")
		}
		client = s.client
		clientConfig = s.clientConfig
	}

	sourcePath, err := force.EvalString(ctx, s.source.Path)
	if err != nil {
		return trace.Wrap(err)
	}

	destPath, err := force.EvalString(ctx, s.destination.Path)
	if err != nil {
		return trace.Wrap(err)
	}

	if s.source.Local && s.destination.Local {
		return trace.BadParameter("source and destination can't be both local")
	}

	w := force.Writer(log)
	defer w.Close()

	// upload
	var cmd scp.Command
	if !s.destination.Local {
		scpConfig := scp.Config{
			User:           clientConfig.User,
			ProgressWriter: w,
			RemoteLocation: destPath,
			Flags: scp.Flags{
				Target: []string{sourcePath},
			},
		}

		cmd, err = scp.CreateUploadCommand(scpConfig)
		if err != nil {
			return trace.Wrap(err)
		}

	} else {
		// download
		scpConfig := scp.Config{
			User:           clientConfig.User,
			ProgressWriter: w,
			RemoteLocation: sourcePath,
			Flags: scp.Flags{
				Target: []string{destPath},
			},
		}

		cmd, err = scp.CreateDownloadCommand(scpConfig)
		if err != nil {
			return trace.Wrap(err)
		}
	}

	log.Infof("Copy: from %v to %v.", sourcePath, destPath)

	return ExecuteSCP(log, client, cmd)
}

// MarshalCode marshals the action into code representation
func (s *CopyAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		Fn:      Copy,
		Args:    []interface{}{s.source, s.destination},
	}
	return call.MarshalCode(ctx)
}

func (s *CopyAction) String() string {
	return fmt.Sprintf("Copy()")
}

// ExecuteSCP runs remote scp command(shellCmd) on the remote server and
// runs local scp handler using SCP Command
func ExecuteSCP(log force.Logger, client *ssh.Client, cmd scp.Command) error {
	shellCmd, err := cmd.GetRemoteShellCmd()
	if err != nil {
		return trace.Wrap(err)
	}

	s, err := client.NewSession()
	if err != nil {
		return trace.Wrap(err)
	}
	defer s.Close()

	stdin, err := s.StdinPipe()
	if err != nil {
		return trace.Wrap(err)
	}

	stdout, err := s.StdoutPipe()
	if err != nil {
		return trace.Wrap(err)
	}

	ch := force.NewPipeNetConn(
		stdout,
		stdin,
		force.MultiCloser(),
		&net.IPAddr{},
		&net.IPAddr{},
	)

	closeC := make(chan interface{}, 1)
	go func() {
		if err = cmd.Execute(ch); err != nil {
			log.WithError(err).Errorf("Failed to execute.")
		}
		stdin.Close()
		close(closeC)
	}()

	runErr := s.Run(shellCmd)
	<-closeC

	if runErr != nil && (err == nil || trace.IsEOF(err)) {
		err = runErr
	}
	if trace.IsEOF(err) {
		err = nil
	}
	return trace.Wrap(err)
}
