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
	Process() Process
	Context() context.Context
	WithValue(key string, value interface{}) ExecutionContext
}
