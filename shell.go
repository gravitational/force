package force

import (
	"os"
	"os/exec"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// Shell runs shell
func Shell(script string) (Action, error) {
	if script == "" {
		return nil, trace.BadParameter("missing script value")
	}
	return &ShellAction{
		Command: script,
	}, nil
}

type ShellAction struct {
	Command string
}

const (
	// ExitCode is a shell exit code
	ExitCode = "Shell.ExitCode"
)

func (s *ShellAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	log.Debugf("Running %q", s.Command)
	cmd := exec.CommandContext(ctx.Context(), "/bin/sh", "-c", s.Command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return ctx.WithValue(ExitCode, cmd.ProcessState.ExitCode()), err
}

// Sequence groups sequence of commands together,
// if one fails, the chain stop execution
func Sequence(actions ...Action) Action {
	return &SequenceAction{
		Actions: actions,
	}
}

type SequenceAction struct {
	Actions []Action
}

func (s *SequenceAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	var err error
	for _, action := range s.Actions {
		ctx, err = action.Run(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	return ctx, err
}

// Pass groups sequence of commands together,
// but, if one fails, the next will continue
func Pass(actions ...Action) Action {
	return &PassAction{
		Actions: actions,
	}
}

type PassAction struct {
	Actions []Action
}

func (s *PassAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	var err error
	for _, action := range s.Actions {
		ctx, err = action.Run(ctx)
	}
	return ctx, err
}
