package force

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/gravitational/trace"
)

// Command
func Command(script string) (Action, error) {
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

func (s *ShellAction) Run(ctx context.Context) error {
	fmt.Printf("Running %v\n", s.Command)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", s.Command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Sequence groups sequence of commands together
func Sequence(actions ...Action) Action {
	return &SequenceAction{
		Actions: actions,
	}
}

type SequenceAction struct {
	Actions []Action
}

func (s *SequenceAction) Run(ctx context.Context) error {
	for _, action := range s.Actions {
		if err := action.Run(ctx); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}
