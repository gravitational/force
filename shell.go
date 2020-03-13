package force

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/gravitational/trace"
)

// ExpectEnv returns environment var by key or
// error if variable not defined
func ExpectEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", trace.NotFound("define environment variable %q", key)
	}
	return val, nil
}

// Script is a shell script
type Script struct {
	// ExportEnv exports all variables from host environment
	ExportEnv bool
	// EchoArgs logs arguments
	EchoArgs bool
	// Command is an inline script, shortcut for
	// /bin/sh -c args...
	Command string
	// Args is a list of arguments, if supplied
	// used instead of the command
	Args []string
	// WorkingDir is a working directory
	WorkingDir string
	// Env is a list of key value environment variables
	Env []string
}

// CheckAndSetDefaults checks and sets default values
func (s *Script) CheckAndSetDefaults(ctx ExecutionContext) error {
	if s.Command == "" && len(s.Args) == 0 {
		return trace.BadParameter("provide either Script{Command: ``} parameter or Script{Args: Strings(...)} parameters")
	}
	if s.Command != "" && len(s.Args) != 0 {
		return trace.BadParameter("provide either Script{Command: ``} parameter or Script{Args: Strings(...)} parameters")
	}
	return nil
}

// Command is a shortcut for shell action
func Command(ctx ExecutionContext, cmd string) (string, error) {
	script := Script{
		Command: cmd,
		// TODO(klizhentas) consider other routes as this is a potential
		// security risk
		ExportEnv: true,
	}
	return script.Run(ctx)
}

// Shell runs shell script
func Shell(ctx ExecutionContext, s Script) (string, error) {
	return s.Run(ctx)
}

// Run runs shell script and returns output as a string
func (s *Script) Run(ctx ExecutionContext) (string, error) {
	w := Log(ctx).Writer()
	defer w.Close()
	buf := NewSyncBuffer()
	err := s.run(ctx, io.MultiWriter(w, buf))
	return buf.String(), trace.Wrap(err)
}

// run runs shell action and captures stdout and stderr to writer
func (s *Script) run(ctx ExecutionContext, w io.Writer) error {
	if err := s.CheckAndSetDefaults(ctx); err != nil {
		return trace.Wrap(err)
	}
	args := s.Args
	if s.Command != "" {
		args = []string{"/bin/sh", "-c", s.Command}
	}
	if s.EchoArgs {
		fmt.Fprintln(w, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = s.Env
	cmd.Dir = s.WorkingDir
	if s.ExportEnv {
		cmd.Env = append(cmd.Env, os.Environ()...)
	}
	return trace.Wrap(cmd.Run())
}

// Sequence groups sequence of commands together,
// if one fails, the chain stop execution
func Sequence(actions ...Action) (Action, error) {
	if len(actions) == 0 {
		return nil, trace.BadParameter("provide at least one argument for Sequence")
	}
	return &SequenceAction{
		actions: actions,
	}, nil
}

// SequenceAction runs actions in a sequence,
// if the action fails, next actions are not run
type SequenceAction struct {
	actions []Action
}

// Run runs actions in sequence using the passed scope
func (s *SequenceAction) Run(ctx ExecutionContext) error {
	var err error
	var deferred []Action
	for i := range s.actions {
		action := s.actions[i]
		_, isDefer := action.(*DeferAction)
		if isDefer {
			deferred = append(deferred, action)
		}
	}
eval:
	for i := range s.actions {
		action := s.actions[i]
		_, isDefer := action.(*DeferAction)
		if isDefer {
			deferred = append(deferred, action)
			continue
		}
		err = action.Run(ctx)
		SetError(ctx, err)
		if err != nil {
			break eval
		}
	}
	// deferred actions are executed in reverse order
	// when defined, and do not prevent other deferreds from running
	for i := len(deferred) - 1; i >= 0; i-- {
		action := deferred[i]
		err = action.Run(ctx)
		if err != nil {
			SetError(ctx, err)
		}
	}
	return Error(ctx)
}

// Defer defers the action executed in sequence
func Defer(action Action) Action {
	return &DeferAction{
		action: action,
	}
}

// DeferAction runs actions in defer
type DeferAction struct {
	action Action
}

// Run runs deferred action
func (d *DeferAction) Run(ctx ExecutionContext) error {
	return d.action.Run(ctx)
}
