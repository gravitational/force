package force

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/gravitational/trace"
)

// LexicalScope is a lexical scope with variables
type LexicalScope interface {
	// AddDefinition adds variable definition in the current lexical scop
	AddDefinition(name string, v interface{}) error

	// GetDefinition gets a variable defined with DefineVarType
	// not the actual variable value is returned, but a prototype
	// value specifying the type
	GetDefinition(name string) (interface{}, error)

	// Variables returns a list of variable names
	// defined in this scope (and the parent scopes)
	Variables() []string
}

// Group represents a group of processes
type Group interface {
	// LexicalScope is a lexical scope
	// represented by this group
	LexicalScope
	// BroadcastEvents will broadcast events
	// to every process in a process group
	BroadcastEvents() chan<- Event

	// ExitEvent is set and returned when the group stop execution,
	// otherwise is nil, so callers should check for validity
	ExitEvent() ExitEvent

	// Context returns a process group context
	Context() context.Context

	// SetPlugin sets process group-local plugin
	// all setters and getters are thread safe
	SetPlugin(key interface{}, val interface{})

	// GetPlugin returns a process group plugin
	// all setters and getters are thread safe
	GetPlugin(key interface{}) (val interface{}, exists bool)

	// Logger returns a logger associated with this group
	Logger() Logger

	// IsDebug returns a global debug override
	IsDebug() bool
}

// Process is a process that is triggered by the event
type Process interface {
	// Name returns a process name
	Name() string
	// Channel returns a process channel
	Channel() Channel
	// Action returns actions assigned to the process
	Action() Action
	// Events returns a channel that receives events
	Events() chan<- Event
	// Start is a non blocking call
	Start(ctx ExecutionContext) error
	// Runner returns a process group
	// this process belongs to
	Group() Group
	// Done signals that process has completed
	// handling channel events and exited
	Done() <-chan struct{}
	// String returns user friendly process name
	String() string
}

// Channel produces events
type Channel interface {
	Start(ctx context.Context) error
	Events() <-chan Event
	Done() <-chan struct{}
}

// Action is an action triggered as a part of the process
type Action interface {
	// CodeMarshaler allows to marshal action into code
	CodeMarshaler
	// Run runs the action in the context of the worker,
	// could modify the context to add metadata, fields or error
	// sometimes, creates a new execution scope
	Run(ctx ExecutionContext) error
}

// ScopeAction can run in the context of the scope instead of creating
// a new one
type ScopeAction interface {
	Action
	// RunWithScope runs actions in sequence using the passed scope
	RunWithScope(scope ExecutionContext) error
}

// Spec is a process specification
type Spec struct {
	Name    String
	Watch   Channel
	Run     Action
	EventsC chan Event `code:"-"`
	// Group if set, will assign the process to a specific group,
	// otherwise, will be set to the default runner
	Group Group `code:"-"`
}

// processNumber is a helper number to generate
// meaningful process numbers in case if user did not specify one
var processNumber = int64(0)

func (s *Spec) CheckAndSetDefaults() error {
	if s.Name == "" {
		num := atomic.AddInt64(&processNumber, 1)
		host, _ := os.Hostname()
		s.Name = String(fmt.Sprintf("%v-%v", host, num))
	}
	if s.Watch == nil {
		oneshot, err := Oneshot()
		if err != nil {
			return trace.Wrap(err)
		}
		s.Watch = oneshot
	}
	if s.Run == nil {
		return trace.BadParameter("the Process needs Spec{Run:} parameter")
	}
	return nil
}

type Event interface {
	// AddMetadata adds metadada to the execution context
	AddMetadata(ctx ExecutionContext)
	// Created returns time when the event was originated in the system
	Created() time.Time
}

// SetError is a helper function that adds an error
// to the context
func SetError(ctx ExecutionContext, err error) {
	if err == nil {
		return
	}
	ctx.SetValue(KeyError, err)
}

// Error is a helper function that finds and returns
// an error
func Error(ctx ExecutionContext) error {
	out := ctx.Value(KeyError)
	if out == nil {
		return nil
	}
	err, ok := out.(error)
	if !ok {
		return nil
	}
	return err
}

// BoolVar is a context bool variable
// that returns a string value from the execution context
type BoolVar interface {
	// Eval evaluates variable and returns bool
	Eval(ctx ExecutionContext) (bool, error)
}

// Bool is a constant bool var
type Bool bool

// Eval evaluates variable and returns bool
func (b Bool) Eval(ctx ExecutionContext) (bool, error) {
	return bool(b), nil
}

// BoolVarFunc wraps function and returns an interface BoolVar
type BoolVarFunc func(ctx ExecutionContext) (bool, error)

// Eval evaluates variable and returns bool
func (f BoolVarFunc) Eval(ctx ExecutionContext) (bool, error) {
	return f(ctx)
}

// IntVar is a context int variable
// that returns a string value from the execution context
type IntVar interface {
	// Eval evaluates variable and returns a string
	Eval(ctx ExecutionContext) (int, error)
}

// Int is a constant int var
type Int int

// MarshalCode marshals the variable to code representation
func (i Int) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return MarshalCode(ctx, int(i))
}

// Value returns int value
func (i Int) Eval(ctx ExecutionContext) (int, error) {
	return int(i), nil
}

func (i *Int) String() string {
	return fmt.Sprintf("%v", int(*i))
}

// IntVarFunc wraps function and returns an interface IntVar
type IntVarFunc func(ctx ExecutionContext) (int, error)

// Eval evaluates value and returns error
func (f IntVarFunc) Eval(ctx ExecutionContext) (int, error) {
	return f(ctx)
}

// StringVar is a context string variable
// that returns a string value from the execution context
type StringVar interface {
	// Eval evaluates variable and returns string
	Eval(ctx ExecutionContext) (string, error)
}

// StringsVar is a context string variable
// that returns a string value from the execution context
type StringsVar interface {
	// Eval evaluates variable and returns string
	Eval(ctx ExecutionContext) ([]string, error)
}

// String is a constant string variable
type String string

// Value evaluates function and returns string
func (s String) Eval(ctx ExecutionContext) (string, error) {
	return string(s), nil
}

// StringVarFunc wraps function and returns an interface StringVar
type StringVarFunc func(ctx ExecutionContext) (string, error)

// Value returns a string value
func (f StringVarFunc) Eval(ctx ExecutionContext) (string, error) {
	return f(ctx)
}

// ID returns a current Force execution ID
func ID() StringVar {
	return &IDAction{}
}

// IDAction returns force ID
type IDAction struct {
}

// Eval evaluates id of the current execution context
func (i *IDAction) Eval(ctx ExecutionContext) (string, error) {
	return ctx.ID(), nil
}

// MarshalCode marshals ID action to code
func (i *IDAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(ID).MarshalCode(ctx)
}

// CloserFunc wraps function
// to create io.Closer compatible type
type CloserFunc func() error

// Close closes resources
func (fn CloserFunc) Close() error {
	return fn()
}
