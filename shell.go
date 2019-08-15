package force

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/gravitational/trace"
)

// NewTrimSpace trims space and newlines
func NewTrimSpace(s StringVar) *TrimSpace {
	return &TrimSpace{
		s: s,
	}
}

// TrimSpace trims space
type TrimSpace struct {
	s StringVar
}

// Eval trims space
func (n *TrimSpace) Eval(ctx ExecutionContext) (string, error) {
	v, err := n.s.Eval(ctx)
	if err != nil {
		return "", trace.Wrap(err)
	}
	return strings.TrimSpace(v), nil
}

// EvalStringVars evaluates string vars and returns a slice of strings
func EvalStringVars(ctx ExecutionContext, in []StringVar) ([]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]string, len(in))
	var err error
	for i, v := range in {
		if v == nil {
			out[i] = ""
		} else {
			out[i], err = v.Eval(ctx)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		}
	}
	return out, nil
}

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

// NewTempDir returns new temp dir
func NewTempDir() interface{} {
	return &TempDir{}
}

type TempDir struct {
}

func (c *TempDir) Eval(ctx ExecutionContext) (string, error) {
	log := Log(ctx)
	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return "", trace.ConvertSystemError(err)
	}
	log.Infof("Created temporary directory %v.", dir)
	return dir, nil
}

// NewRemoveDir returns a n action rm
func NewRemoveDir(dir StringVar) interface{} {
	return &RemoveDir{dir: dir}
}

type RemoveDir struct {
	dir StringVar
}

func (c *RemoveDir) Run(ctx ExecutionContext) error {
	dir, err := c.dir.Eval(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	if dir == "" {
		return trace.BadParameter("empty dir to remove")
	}
	log := Log(ctx)
	err = os.RemoveAll(dir)
	if err != nil {
		return trace.ConvertSystemError(err)
	}
	log.Infof("Removed directory path %v.", dir)
	return nil
}

// Script is a shell script
type Script struct {
	// Command is an inline script, shortcut for
	// /bin/sh -c args...
	Command StringVar
	// Args is a list of arguments, if supplied
	// used instead of the command
	Args []StringVar
	// WorkingDir is a working directory
	WorkingDir StringVar
}

// CheckAndSetDefaults checks and sets default values
func (s *Script) CheckAndSetDefaults(ctx ExecutionContext) error {
	command, err := EvalString(ctx, s.Command)
	if err != nil {
		return trace.Wrap(err)
	}
	args, err := EvalStringVars(ctx, s.Args)
	if err != nil {
		return trace.Wrap(err)
	}
	if command == "" && len(args) == 0 {
		return trace.BadParameter("provide either Script{Command: ``} parameter or Script{Args: Strings(...)} parameters")
	}
	if command != "" && len(args) != 0 {
		return trace.BadParameter("provide either Script{Command: ``} parameter or Script{Args: Strings(...)} parameters")
	}
	_, err = EvalString(ctx, s.WorkingDir)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// Shell runs shell script
func Shell(s Script) (Action, error) {
	return &ShellAction{
		script: s,
	}, nil
}

// ShellAction runs shell script
type ShellAction struct {
	script Script
}

// Run runs the script
func (s *ShellAction) Run(ctx ExecutionContext) error {
	w := Writer(Log(ctx))
	defer w.Close()
	return s.run(ctx, w)
}

// Eval runs shell script and returns output as a string
func (s *ShellAction) Eval(ctx ExecutionContext) (string, error) {
	buf := NewSyncBuffer()
	err := s.run(ctx, buf)
	return buf.String(), trace.Wrap(err)
}

// run runs shell action and captures stdout and stderr to writer
func (s *ShellAction) run(ctx ExecutionContext, w io.Writer) error {
	if err := s.script.CheckAndSetDefaults(ctx); err != nil {
		return trace.Wrap(err)
	}
	args, err := EvalStringVars(ctx, s.script.Args)
	if err != nil {
		return trace.Wrap(err)
	}
	command, err := EvalString(ctx, s.script.Command)
	if err != nil {
		return trace.Wrap(err)
	}
	if command != "" {
		args = []string{"/bin/sh", "-c", command}
	}
	workingDir, err := EvalString(ctx, s.script.WorkingDir)
	if err != nil {
		return trace.Wrap(err)
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Dir = workingDir
	return trace.Wrap(cmd.Run())
}

func (s *ShellAction) String() string {
	return fmt.Sprintf("Shell()")
}

// NewParallel creates a new series of actions executed in parallel
type NewParallel struct {
}

// NewInstance returns a new instance
func (n *NewParallel) NewInstance(group Group) (Group, interface{}) {
	return WithLexicalScope(group), Parallel
}

// Parallel runs actions in parallel
func Parallel(actions ...Action) Action {
	return &ParallelAction{
		Actions: actions,
	}
}

// ParallelAction runs actions in parallel
// waits for all to complete, if any of them fail,
// returns error
type ParallelAction struct {
	Actions []Action
}

// Run runs actions in parallel
func (p *ParallelAction) Run(ctx ExecutionContext) error {
	scopeCtx := WithRuntimeScope(ctx)
	errC := make(chan error, len(p.Actions))
	for _, action := range p.Actions {
		go p.runAction(scopeCtx, action, errC)
	}
	var errors []error
	for i := 0; i < len(p.Actions); i++ {
		select {
		case err := <-errC:
			if err != nil {
				Log(ctx).WithError(err).Warningf("Action %v has failed.", p.Actions[i])
				errors = append(errors, err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if len(errors) > 0 {
		SetError(ctx, trace.NewAggregate(errors...))
	}
	return Error(ctx)
}

func (p *ParallelAction) runAction(ctx ExecutionContext, action Action, errC chan error) {
	err := action.Run(ctx)
	select {
	case errC <- err:
	case <-ctx.Done():
	}
}

// Defer defers the action executed in sequence
func Defer(action Action) Action {
	return &DeferAction{
		Action: action,
	}
}

// DeferAction runs actions in defer
type DeferAction struct {
	Action Action
}

// Run runs deferred action
func (d *DeferAction) Run(ctx ExecutionContext) error {
	return d.Action.Run(ctx)
}

// NewSequence creates a new sequence
// with a new lexical scope
type NewSequence struct {
}

// NewInstance returns a new instance of a function with a new lexical scope
func (n *NewSequence) NewInstance(group Group) (Group, interface{}) {
	return WithLexicalScope(group), Sequence
}

// Sequence groups sequence of commands together,
// if one fails, the chain stop execution
func Sequence(actions ...Action) Action {
	return &SequenceAction{
		Actions: actions,
	}
}

// SequenceAction runs actions in a sequence,
// if the action fails, next actions are not run
type SequenceAction struct {
	Actions []Action
}

// Run runs actions in sequence
func (s *SequenceAction) Run(ctx ExecutionContext) error {
	scopeCtx := WithRuntimeScope(ctx)
	var err error
	var deferred []Action
	for i := range s.Actions {
		action := s.Actions[i]
		_, isDefer := action.(*DeferAction)
		if isDefer {
			deferred = append(deferred, action)
			continue
		}
		err = action.Run(scopeCtx)
		SetError(ctx, err)
		if err != nil {
			return trace.Wrap(err)
		}
	}
	// deferred actions are executed in reverse order
	// when defined, and do not prevent other deferreds from running
	for i := len(deferred) - 1; i >= 0; i-- {
		action := deferred[i]
		err = action.Run(scopeCtx)
		if err != nil {
			SetError(ctx, err)
		}
	}
	return Error(ctx)
}

// NewContinue creates a new continued sequence
// with a new lexical scope
type NewContinue struct {
}

// NewInstance returns a new instance of a function with a new lexical scope
func (n *NewContinue) NewInstance(group Group) (Group, interface{}) {
	return WithLexicalScope(group), Sequence
}

// Continue runs actions one by one,
// if one fails, it will continue running others
func Continue(actions ...Action) Action {
	return &ContinueAction{
		Actions: actions,
	}
}

// ContinueAction runs actions
type ContinueAction struct {
	Actions []Action
}

func (s *ContinueAction) Run(ctx ExecutionContext) error {
	scopeCtx := WithRuntimeScope(ctx)
	for _, action := range s.Actions {
		SetError(ctx, action.Run(scopeCtx))
	}
	return Error(ctx)
}

// Test is a struct used for tests
type Test struct {
	// I is an integer variable
	I IntVar
	// S is a string variable
	S StringVar
	// B is a bool variable
	B BoolVar
}
