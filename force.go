package force

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"time"

	"github.com/gravitational/trace"
)

// Group represents a group of processes
type Group interface {
	// BroadcastEvents will broadcast events
	// to every process in a process group
	BroadcastEvents() chan<- Event

	// ExitEvent is set and returned when the group stop execution,
	// otherwise is nil, so callers should check for validity
	ExitEvent() ExitEvent

	// Context returns a process group context
	Context() context.Context

	// SetVar sets process group-local variable
	// all setters and getters are thread safe
	SetVar(key interface{}, val interface{})

	// GetVar returns a process group local variable
	// all setters and getters are thread safe
	GetVar(key interface{}) (val interface{}, exists bool)

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
	Start(ctx context.Context) error
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
	// Run runs the action in the context of the worker,
	// could modify the context to add metadata, fields or error
	Run(ctx ExecutionContext) error
}

// Spec is a process specification
type Spec struct {
	Name    String
	Watch   Channel
	Run     Action
	EventsC chan Event
	// Group if set, will assign the process to a specific group,
	// otherwise, will be set to the default runner
	Group Group
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
		return trace.BadParameter("the Process is missing Spec{Watch:}, in case if you need an unconditional execution, use Spec{Watch: Oneshot()}")
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

// ExecutionContext represents an execution context
// of a certain action execution chain,
type ExecutionContext interface {
	context.Context
	// Event is an event that generated the action
	Event() Event
	// Process returns a process associated with context
	Process() Process
	// SetValue adds a key value pair to the context
	SetValue(key interface{}, value interface{})
	// ID is an execution unique identifier
	ID() string
	// AddCloser adds closer to the context
	AddCloser(io.Closer)
	// Close closes the context
	// and releases all associated resources
	// registered with Closer
	Close() error
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

// IntVar is a context int variable
// that returns a string value from the execution context
type IntVar interface {
	// Value returns a string
	Value(ctx ExecutionContext) int
}

// Int is a constant int var
type Int int

// Value returns int value
func (i Int) Value(ctx ExecutionContext) int {
	return int(i)
}

// StringVar is a context string variable
// that returns a string value from the execution context
type StringVar interface {
	// Value returns a string
	Value(ctx ExecutionContext) string
}

// String is a constant string variable
type String string

// Value returns a string value
func (s String) Value(ctx ExecutionContext) string {
	return string(s)
}

// StringVarFunc wraps function and returns an interface VarString
type StringVarFunc func(ctx ExecutionContext) string

// Value returns a string value
func (f StringVarFunc) Value(ctx ExecutionContext) string {
	return f(ctx)
}

// ID returns a current Force execution ID
func ID() StringVar {
	return StringVarFunc(func(ctx ExecutionContext) string {
		return ctx.ID()
	})
}

// CloserFunc wraps function
// to create io.Closer compatible type
type CloserFunc func() error

// Close closes resources
func (fn CloserFunc) Close() error {
	return fn()
}
