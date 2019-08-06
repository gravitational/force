package force

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/gravitational/trace"
)

// Env returns environment variable
func Env(key String) String {
	return String(os.Getenv(string(key)))
}

// ExpectEnv returns environment var by key or
// error if variable not defined
func ExpectEnv(key String) (String, error) {
	val := os.Getenv(string(key))
	if val == "" {
		return String(""), trace.NotFound("define environment variable %q", key)
	}
	return String(val), nil
}

// Strings returns a list of strings evaluated from the arguments
func Strings(args ...interface{}) ([]StringVar, error) {
	out := make([]StringVar, len(args))
	for i := range args {
		switch v := args[i].(type) {
		case StringVar:
			out[i] = v
		case string:
			out[i] = String(v)
		default:
			return nil, trace.BadParameter("argument %q is not a string", args[i])
		}
	}
	return out, nil
}

// Sprintf is jsut like Sprintf
func Sprintf(format String, args ...interface{}) StringVar {
	return StringVarFunc(func(ctx ExecutionContext) string {
		eval := make([]interface{}, len(args))
		for i := range args {
			eval[i] = Eval(ctx, args[i])
		}
		return fmt.Sprintf(string(format), eval...)
	})
}

// Var returns string variable
func Var(name String) StringVar {
	return StringVarFunc(func(ctx ExecutionContext) string {
		val, ok := ctx.Value(ContextKey(string(name))).(string)
		if !ok {
			Log(ctx).Errorf("Failed to convert variable %q to string from %T, returning empty value.", name, name)
			return ""
		}
		return val
	})
}

// Define creates a define variable action
func Define(name String, value interface{}) (Action, error) {
	if name == "" {
		return nil, trace.BadParameter("Define needs variable name")
	}
	if value == nil {
		return nil, trace.BadParameter("nils are not supported here because they are evil")
	}
	return &DefineAction{
		name:  string(name),
		value: value,
	}, nil
}

// DefineAction defines a variable
type DefineAction struct {
	name  string
	value interface{}
}

// Eval evaluates variable based on the execution context
func Eval(ctx ExecutionContext, variable interface{}) interface{} {
	switch v := variable.(type) {
	case StringVar:
		return v.Value(ctx)
	default:
		return v
	}
}

// Run defines a variable on the context
func (p *DefineAction) Run(ctx ExecutionContext) error {
	v := ctx.Value(ContextKey(p.name))
	if v != nil {
		return trace.AlreadyExists("variable %q is already defined", p.name)
	}
	ctx.SetValue(ContextKey(p.name), Eval(ctx, p.value))
	return nil
}

// WithTempDir creates a new temp dir, and defines a variable
// with a given name
func WithTempDir(name String, actions ...Action) (Action, error) {
	if name == "" {
		return nil, trace.BadParameter("TempDir needs name")
	}
	return &WithTempDirAction{
		name:    string(name),
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

// WithChangeDir sets the current working directory
// and executes the actions in a sequence
func WithChangeDir(name StringVar, actions ...Action) (Action, error) {
	if name == nil {
		return nil, trace.BadParameter("WithChangeDir needs a directory name")
	}
	return &WithChangeDirAction{
		name:    name,
		actions: actions,
	}, nil
}

// WithChangeDirAction sets the current directory value
type WithChangeDirAction struct {
	name    StringVar
	actions []Action
}

// Run runs with temp dir action
func (p *WithChangeDirAction) Run(ctx ExecutionContext) error {
	log := Log(ctx)
	name := p.name.Value(ctx)
	if name == "" {
		return trace.BadParameter("WithChangeDir executed with empty string on %v", name)
	}
	fi, err := os.Stat(name)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	if !fi.IsDir() {
		return trace.BadParameter("%q is not a directory", name)
	}
	ctx.SetValue(KeyCurrentDir, name)
	log.Infof("Changing current dir to %q.", name)
	defer func() {
		ctx.SetValue(KeyCurrentDir, nil)
	}()
	err = Sequence(p.actions...).Run(ctx)
	return trace.Wrap(err)
}

// Shell runs shell
func Shell(script StringVar) (Action, error) {
	if script == nil {
		return nil, trace.BadParameter("missing script value")
	}
	return &ShellAction{
		command: script,
	}, nil
}

type ShellAction struct {
	command StringVar
}

func (s *ShellAction) Run(ctx ExecutionContext) error {
	log := Log(ctx)
	command := s.command.Value(ctx)
	curDirI := ctx.Value(KeyCurrentDir)
	var curDir string
	if curDirI != nil {
		curDir, _ = curDirI.(string)
		log.Infof("Running %q with current directory set to %v.", command, curDirI)
	} else {
		log.Infof("Running %q.", command)
	}
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)
	w := Writer(log)
	defer w.Close()
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Dir = curDir
	err := cmd.Run()
	return trace.Wrap(err)
}

func (s *ShellAction) String() string {
	return fmt.Sprintf("Shell()")
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
