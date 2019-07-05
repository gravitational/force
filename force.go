package force

import (
	"context"
)

// Proc is a process that is triggered by the event
type Proc interface {
	Channel() Channel
	Action() Action
	// Events returns a channel to receive events
	Events() chan<- Event
	// Start is a non blocking call
	Start(ctx context.Context) error
}

// Channel produces events
type Channel interface {
	Start(ctx context.Context) error
	Events() <-chan Event
	Done() <-chan struct{}
}

// Action is an action triggered as a part of the process
type Action interface {
	// Run returns runner
	Run(ctx context.Context) error
}

// Spec is a process spec
type Spec struct {
	Name    string
	Watch   Channel
	Run     Action
	EventsC chan Event
}

type Event interface {
}
