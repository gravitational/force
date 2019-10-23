package force

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"

	"github.com/gravitational/trace"
)

// EvalString evaluates string from var
func EvalString(ctx ExecutionContext, in Expression) (string, error) {
	if in == nil {
		return "", nil
	}
	out, err := in.Eval(ctx)
	if err != nil {
		return "", trace.Wrap(err)
	}
	s, ok := out.(string)
	if !ok {
		return "", trace.BadParameter("expected string got %T", out)
	}
	return s, nil
}

// EvalBool evaluates bool from var
func EvalBool(ctx ExecutionContext, in Expression) (bool, error) {
	out, err := in.Eval(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}
	b, ok := out.(bool)
	if !ok {
		return false, trace.BadParameter("expected bool got %T", out)
	}
	return b, nil
}

// EvalInt evaluates int from var
func EvalInt(ctx ExecutionContext, in Expression) (int, error) {
	out, err := in.Eval(ctx)
	if err != nil {
		return 0, trace.Wrap(err)
	}
	i, ok := out.(int)
	if !ok {
		return 0, trace.BadParameter("expected bool got %T", out)
	}
	return i, nil
}

// EvalStringVars evaluates string vars and returns a slice of strings
func EvalStringVars(ctx ExecutionContext, in []Expression) ([]string, error) {
	if in == nil {
		return nil, nil
	}
	out := make([]string, len(in))
	for i, v := range in {
		if v == nil {
			out[i] = ""
		} else {
			iface, err := v.Eval(ctx)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			s, ok := iface.(string)
			if !ok {
				return nil, trace.BadParameter("expected string, got %T", iface)
			}
			out[i] = s
		}
	}
	return out, nil
}

// ExpectEnv returns environment var by key or
// error if variable not defined
func ExpectEnv(key string) (string, error) {
	val := os.Getenv(key)
	if val == "" {
		return "", trace.NotFound("define environment variable %q", key)
	}
	return val, nil
}

// ExpressionSlice is a wrapper around
// a slice of expressoins that adds interface
// method to evaluate to expression
type ExpressionSlice []Expression

// Eval evaluates a list of var references to types
func (s ExpressionSlice) Eval(ctx ExecutionContext) (interface{}, error) {
	values := reflect.MakeSlice(reflect.TypeOf(s.Type()), len(s), len(s))
	for i := range s {
		v, err := s[i].Eval(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		values.Index(i).Set(reflect.ValueOf(v))
	}
	return values.Interface(), nil
}

// Type of slice of expressions
func (s ExpressionSlice) Type() interface{} {
	if len(s) == 0 {
		return []interface{}{}
	}
	sliceType := reflect.SliceOf(reflect.TypeOf(s[0].Type()))
	return reflect.Zero(sliceType).Interface()
}

// Vars returns string vars
func (s ExpressionSlice) Vars() []Expression {
	return []Expression(s)
}

// StringSlice represents a slice of strings
type StringSlice []Expression

// Eval evaluates a list of var references to types
func (s StringSlice) Eval(ctx ExecutionContext) (interface{}, error) {
	return ExpressionSlice(s).Eval(ctx)
}

// MarshalCode
func (s StringSlice) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Strings,
		Args: make([]interface{}, len(s)),
	}
	for i := 0; i < len(s); i++ {
		call.Args[i] = s[i]
	}
	return call.MarshalCode(ctx)
}

// Type of slice of expressions
func (s StringSlice) Type() interface{} {
	return []string{}
}

// Vars returns string vars
func (s StringSlice) Vars() []Expression {
	return []Expression(s)
}

// IntSlice represents a slice of integers
type IntSlice []Expression

// MarshalCode
func (s IntSlice) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return MarshalCode(ctx, []Expression(s))
}

// Eval evaluates a list of var references to types
func (s IntSlice) Eval(ctx ExecutionContext) (interface{}, error) {
	return ExpressionSlice(s).Eval(ctx)
}

// Type of slice of expressions
func (s IntSlice) Type() interface{} {
	return []int{}
}

// Vars returns string vars
func (s IntSlice) Vars() []Expression {
	return []Expression(s)
}

// BoolSlice represents a slice of integers
type BoolSlice []Expression

// MarshalCode
func (s BoolSlice) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return MarshalCode(ctx, []Expression(s))
}

// Eval evaluates a list of var references to types
func (s BoolSlice) Eval(ctx ExecutionContext) (interface{}, error) {
	return ExpressionSlice(s).Eval(ctx)
}

// Type of slice of expressions
func (s BoolSlice) Type() interface{} {
	return []bool{}
}

// Vars returns string vars
func (s BoolSlice) Vars() []Expression {
	return []Expression(s)
}

// Strings returns a list of strings evaluated from the arguments
func Strings(args ...Expression) (Expression, error) {
	out := make([]Expression, len(args))
	for i := range args {
		if err := ExpectString(args[i]); err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = args[i]
	}
	return StringSlice(out), nil
}

// Script is a shell script
type Script struct {
	// ExportEnv exports all variables from host environment
	ExportEnv Expression
	// Command is an inline script, shortcut for
	// /bin/sh -c args...
	Command Expression
	// Args is a list of arguments, if supplied
	// used instead of the command
	Args []Expression
	// WorkingDir is a working directory
	WorkingDir Expression
	// Env is a list of key value environment variables
	Env []Expression
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

// Command is a shortcut for shell action
func Command(cmd Expression) (Action, error) {
	if err := ExpectString(cmd); err != nil {
		return nil, trace.Wrap(err)
	}
	return &ShellAction{
		script: Script{
			Command: cmd,
			// TODO(klizhentas) consider other routes as this is a potential
			// security risk
			ExportEnv: Bool(true),
		},
	}, nil
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

func (s *ShellAction) Type() interface{} {
	return ""
}

// Eval runs shell script and returns output as a string
func (s *ShellAction) Eval(ctx ExecutionContext) (interface{}, error) {
	w := Writer(Log(ctx))
	defer w.Close()
	buf := NewSyncBuffer()
	err := s.run(ctx, io.MultiWriter(w, buf))
	return buf.String(), trace.Wrap(err)
}

// MarshalCode marshals action into code representation
func (s *ShellAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Shell,
		Args: []interface{}{s.script},
	}
	return call.MarshalCode(ctx)
}

// run runs shell action and captures stdout and stderr to writer
func (s *ShellAction) run(ctx ExecutionContext, w io.Writer) error {
	if err := s.script.CheckAndSetDefaults(ctx); err != nil {
		return trace.Wrap(err)
	}
	exportEnv, err := EvalBool(ctx, s.script.ExportEnv)
	if err != nil {
		return trace.Wrap(err)
	}
	args, err := EvalStringVars(ctx, s.script.Args)
	if err != nil {
		return trace.Wrap(err)
	}
	env, err := EvalStringVars(ctx, s.script.Env)
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
	cmd.Env = env
	cmd.Dir = workingDir
	if exportEnv {
		cmd.Env = append(cmd.Env, os.Environ()...)
	}
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
func Parallel(actions ...Action) (Action, error) {
	if len(actions) == 0 {
		return nil, trace.BadParameter("provide at least one argument for Parallel")
	}
	t := actions[0].Type()
	for _, a := range actions {
		if err := ExpectEqualTypes(a.Type(), t); err != nil {
			return nil, trace.BadParameter("all arguments in Parallel should evaluate to the same type %v, %v", t, err)
		}
	}
	return &ParallelAction{
		actions: actions,
	}, nil
}

// ParallelAction runs actions in parallel
// waits for all to complete, if any of them fail,
// returns error
type ParallelAction struct {
	actions []Action
}

// Type returns a slice of actions' results
func (p *ParallelAction) Type() interface{} {
	elementType := reflect.TypeOf(p.actions[0].Type())
	sliceType := reflect.SliceOf(elementType)
	return reflect.Zero(sliceType).Interface()
}

type result struct {
	err   error
	value interface{}
}

// Eval runs actions in parallel and returns a slice of results
func (p *ParallelAction) Eval(ctx ExecutionContext) (interface{}, error) {
	scopeCtx := WithRuntimeScope(ctx)
	resultsC := make(chan result, len(p.actions))
	for _, action := range p.actions {
		go p.runAction(scopeCtx, action, resultsC)
	}
	var errors []error
	values := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(p.actions[0].Type())), len(p.actions), len(p.actions))
	for i := 0; i < len(p.actions); i++ {
		select {
		case out := <-resultsC:
			if out.err != nil {
				Log(ctx).WithError(out.err).Warningf("Action %v has failed.", p.actions[i])
				errors = append(errors, out.err)
			} else {
				values.Index(i).Set(reflect.ValueOf(out.value))
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if len(errors) > 0 {
		SetError(ctx, trace.NewAggregate(errors...))
	}
	return values.Interface(), Error(ctx)
}

// MarshalCode marshals action into code representation
func (p *ParallelAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Parallel,
		Args: make([]interface{}, len(p.actions)),
	}
	for i := range p.actions {
		call.Args[i] = p.actions[i]
	}
	return call.MarshalCode(ctx)
}

func (p *ParallelAction) runAction(ctx ExecutionContext, action Action, errC chan result) {
	value, err := action.Eval(ctx)
	select {
	case errC <- result{value: value, err: err}:
	case <-ctx.Done():
	}
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

func (d *DeferAction) Type() interface{} {
	return d.action.Type()
}

// Run runs deferred action
func (d *DeferAction) Eval(ctx ExecutionContext) (interface{}, error) {
	return d.action.Eval(ctx)
}

// MarshalCode marshals action into code representation
func (d *DeferAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Defer, d.action).MarshalCode(ctx)
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
func Sequence(actions ...Action) (ScopeAction, error) {
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

func (p *SequenceAction) Type() interface{} {
	return p.actions[len(p.actions)-1].Type()
}

// MarshalCode marshals action into code representation
func (p *SequenceAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Sequence,
		Args: make([]interface{}, len(p.actions)),
	}
	for i := range p.actions {
		call.Args[i] = p.actions[i]
	}
	return call.MarshalCode(ctx)
}

// RunWithScope runs actions in sequence using the passed scope
func (s *SequenceAction) EvalWithScope(ctx ExecutionContext) (interface{}, error) {
	var err error
	var deferred []Action
	var last interface{}
	for i := range s.actions {
		action := s.actions[i]
		_, isDefer := action.(*DeferAction)
		if isDefer {
			deferred = append(deferred, action)
			continue
		}
		last, err = action.Eval(ctx)
		SetError(ctx, err)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}
	// deferred actions are executed in reverse order
	// when defined, and do not prevent other deferreds from running
	for i := len(deferred) - 1; i >= 0; i-- {
		action := deferred[i]
		_, err = action.Eval(ctx)
		if err != nil {
			SetError(ctx, err)
		}
	}
	return last, Error(ctx)
}

// Eval evaluates actions in sequence
func (s *SequenceAction) Eval(ctx ExecutionContext) (interface{}, error) {
	return s.EvalWithScope(WithRuntimeScope(ctx))
}

// Test is a struct used for tests
type Test struct {
	// I is an integer variable
	I Expression
	// S is a string variable
	S Expression
	// B is a bool variable
	B Expression
}
