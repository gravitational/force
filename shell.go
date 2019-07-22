package force

import (
	"fmt"
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

func (s *ShellAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	log.Debugf("Running %q.", s.Command)
	cmd := exec.CommandContext(ctx.Context(), "/bin/sh", "-c", s.Command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ctx, nil
}

func (s *ShellAction) String() string {
	return fmt.Sprintf("Shell(command=%v)", s.Command)
}

// Chain groups sequence of commands together,
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
		newCtx, err := action.Run(ctx)
		// context was updated, with some metadata, update it
		if newCtx != nil {
			ctx = newCtx
		}
		// error is not nil, stop sequence execution
		if err != nil {
			ctx = WithError(ctx, err)
			return ctx, trace.Wrap(err)
		}
	}
	return ctx, err
}

// Continue groups sequence of commands together,
// but, if one fails, the next will continue
func Continue(actions ...Action) Action {
	return &ContinueAction{
		Actions: actions,
	}
}

type ContinueAction struct {
	Actions []Action
}

func (s *ContinueAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	var err error
	for _, action := range s.Actions {
		log.Debugf("Running action %v.", action)
		newCtx, err := action.Run(ctx)
		// context was updated, with some metadata, update it
		if newCtx != nil {
			ctx = newCtx
		}
		// error is not nil, continue sequence execution
		if err != nil {
			ctx = WithError(ctx, err)
		}
	}
	return ctx, err
}
