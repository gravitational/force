package force

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/gravitational/trace"
)

// Var returns string variable
func Var(name string) StringVar {
	return StringVarFunc(func(ctx ExecutionContext) string {
		val, ok := ctx.Value(ContextKey(name)).(string)
		if !ok {
			Log(ctx).Errorf("Failed to convert variable %q to string from %T, returning empty value.", name, name)
			return ""
		}
		return val
	})
}

// WithTempDir creates a new temp dir, and defines a variable
// with a given name
func WithTempDir(name string, actions ...Action) (Action, error) {
	if name == "" {
		return nil, trace.BadParameter("TempDir needs name")
	}
	return &WithTempDirAction{
		name:    name,
		actions: actions,
	}, nil
}

// WithTempDirAction creates one or many temporary directories,
// executes action and deletes the temp directory later
type WithTempDirAction struct {
	name    string
	actions []Action
}

// Run runs with temp dir action
func (p *WithTempDirAction) Run(ctx ExecutionContext) error {
	log := Log(ctx)
	dir, err := ioutil.TempDir("", p.name)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	log.Infof("Created temporary directory %q with path %q.", p.name, dir)
	ctx.SetValue(ContextKey(p.name), dir)
	defer func() {
		if err := trace.ConvertSystemError(os.RemoveAll(dir)); err != nil {
			log.Errorf("Failed to delete temporary directory %q with path %q: %v.", p.name, dir, err)
		} else {
			log.Infof("Deleted temporary directory %q with path %q.", p.name, dir)
		}
	}()
	err = Sequence(p.actions...).Run(ctx)
	return trace.Wrap(err)
}

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

func (s *ShellAction) Run(ctx ExecutionContext) error {
	log := Log(ctx)
	log.Infof("Running %q.", s.Command)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", s.Command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	return trace.Wrap(err)
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

func (s *SequenceAction) Run(ctx ExecutionContext) error {
	var err error
	for _, action := range s.Actions {
		err = action.Run(ctx)
		SetError(ctx, err)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	return Error(ctx)
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

func (s *ContinueAction) Run(ctx ExecutionContext) error {
	for _, action := range s.Actions {
		SetError(ctx, action.Run(ctx))
	}
	return Error(ctx)
}
