package force

import (
	"context"
	"io"
	"sync"

	"github.com/gravitational/trace"
)

// ExecutionContext represents an execution context
// of a certain action execution chain,
type ExecutionContext interface {
	context.Context
	// Event is an event that generated the action
	Event() Event
	// Process returns a process associated with context
	Process() Process
	// SetValue sets a key value pair to the context
	SetValue(key interface{}, value interface{}) error
	// ID is an execution unique identifier
	ID() string
	// AddCloser adds closer to the context
	AddCloser(io.Closer)
	// Close closes the context
	// and releases all associated resources
	// registered with Closer
	Close() error
}

// ExecutionScope is a variable scope
// defined during execution
type ExecutionScope interface {
	// SetValue sets a key value pair to the context
	SetValue(key interface{}, value interface{}) error
	// Value returns a value defined in the context
	Value(key interface{}) interface{}
}

// WithRuntimeScope wraps a group to create a new runtime scope
func WithRuntimeScope(ctx ExecutionContext) *RuntimeScope {
	return &RuntimeScope{
		RWMutex:          &sync.RWMutex{},
		ExecutionContext: ctx,
		vars:             make(map[interface{}]interface{}),
	}
}

// RuntimeScope wraps an execution context to create
// a new one with new variable values
type RuntimeScope struct {
	*sync.RWMutex
	ExecutionContext
	vars map[interface{}]interface{}
}

// SetValue sets a key value pair
func (l *RuntimeScope) SetValue(key interface{}, value interface{}) error {
	l.Lock()
	defer l.Unlock()
	if key == nil {
		return trace.BadParameter("provide key value")
	}
	if value == nil {
		return trace.BadParameter("provide key name")
	}
	l.vars[key] = value
	return nil
}

// Value returns a value
func (l *RuntimeScope) Value(key interface{}) interface{} {
	l.RLock()
	out := l.vars[key]
	l.RUnlock()
	if out != nil {
		return out
	}
	if l.ExecutionContext == nil {
		return nil
	}
	return l.ExecutionContext.Value(key)
}
