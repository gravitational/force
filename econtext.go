package force

import (
	"context"
	"sync"
	"time"

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

// NewContext returns a new context
func NewContext(cfg ContextConfig) *Context {
	return &Context{
		RuntimeScope: WithRuntimeScope(nil),
		RWMutex:      &sync.RWMutex{},
		cfg:          cfg,
	}
}

// ContextConfig sets up local context
type ContextConfig struct {
	// Context is a context to wrap
	Context context.Context
	// Process is a process
	Process Process
	// Event is event
	Event Event
	// ID is an execution ID
	ID string
}

// Context implements local execution context
type Context struct {
	cfg ContextConfig
	*sync.RWMutex
	*RuntimeScope
}

// ID is an execution unique identifier
func (c *Context) ID() string {
	return c.cfg.ID
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *Context) Deadline() (deadline time.Time, ok bool) {
	return c.cfg.Context.Deadline()
}

// Done returns channel that is closed when the context is closed
func (c *Context) Done() <-chan struct{} {
	return c.cfg.Context.Done()
}

// Err returns an error associated with the context
// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
// After Err returns a non-nil error, successive calls to Err return the same error.
func (c *Context) Err() error {
	return c.cfg.Context.Err()
}

// Event is an event that generated the action
func (c *Context) Event() Event {
	return c.cfg.Event
}

// Process returns a process associated with the context
func (c *Context) Process() Process {
	return c.cfg.Process
}
