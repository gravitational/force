package force

import (
	"context"
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
}

// Process is a process that is triggered by the event
type Process interface {
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
	// once run, the worker returns a modified execution context
	Run(ctx ExecutionContext) (ExecutionContext, error)
}

// Spec is a process specification
type Spec struct {
	Name    string
	Watch   Channel
	Run     Action
	EventsC chan Event
	// Group if set, will assign the process to a specific group,
	// otherwise, will be set to the default runner
	Group Group
}

type Event interface {
}

// ExecutionContext represents an execution context
// of a certain action execution chain,
type ExecutionContext interface {
	// Event is an event that generated the action
	Event() Event
	Process() Process
	Context() context.Context
	WithValue(key interface{}, value interface{}) ExecutionContext
	Value(key interface{}) interface{}
}

// ContextKey is a special type used to set force-related
// context value, is recommended by context package to use
// separate type for context values to prevent
// namespace clash
type ContextKey string

const (
	// Error is an error value
	Error = ContextKey("error")
)

// WithError is a helper function that wraps execution context
func WithError(ctx ExecutionContext, err error) ExecutionContext {
	if err == nil {
		return ctx
	}
	return ctx.WithValue(Error, err)
}

// GetError is a helper function that finds and returns
// an error
func GetError(ctx ExecutionContext) error {
	out := ctx.Value(Error)
	if out == nil {
		return nil
	}
	err, ok := out.(error)
	if !ok {
		return nil
	}
	return err
}
