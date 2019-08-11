package runner

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// NewLocalProcess starts a new local process
// in the future we may support remote processes,
// e.g. K8s process?
func NewLocalProcess(ctx context.Context, logger force.Logger, spec force.Spec) (*LocalProcess, error) {
	if err := spec.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	return &LocalProcess{
		logger:  logger,
		ctx:     cancelCtx,
		cancel:  cancel,
		Spec:    spec,
		eventsC: make(chan force.Event, 32),
	}, nil
}

// LocalProcess implements a process interface
type LocalProcess struct {
	force.Spec
	eventsC chan force.Event
	ctx     context.Context
	cancel  context.CancelFunc
	logger  force.Logger
}

// EventSource returns channel
func (l *LocalProcess) Channel() force.Channel {
	return l.Watch
}

func (l *LocalProcess) Action() force.Action {
	return l.Run
}

// Done returns a channel that signals that process has completed
// handling channel events and exited
func (l *LocalProcess) Done() <-chan struct{} {
	return l.ctx.Done()
}

func (l *LocalProcess) Events() chan<- force.Event {
	return l.eventsC
}

func (l *LocalProcess) Start(ctx context.Context) error {
	go l.triggerActions(ctx)
	return nil
}

// Runner returns a process group
// this process belongs to
func (l *LocalProcess) Group() force.Group {
	return l.Spec.Group
}

// Name returns a process name
func (l *LocalProcess) Name() string {
	return string(l.Spec.Name)
}

// String returns process user friendly string
func (l *LocalProcess) String() string {
	return fmt.Sprintf("Process %v", l.Spec.Name)
}

// ShortID generates short random ids
func ShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (l *LocalProcess) triggerActions(ctx context.Context) {
	for {
		select {
		case <-l.ctx.Done():
			l.logger.Debugf("This process has exited, returning.")
			return
		case <-ctx.Done():
			l.logger.Debugf("Runner has exited, returning.")
			return
		case event := <-l.eventsC:
			l.logger.Debugf("%v has received %v.", l, event)
			if force.IsExit(event) {
				l.logger.Debugf("Has received an exit event, exiting.")
				l.logger.Debugf("%v has triggered an exit event, exiting.", l)
				l.cancel()
				return
			}
			go func() {
				execContext := &LocalContext{
					RWMutex: &sync.RWMutex{},
					context: ctx,
					process: l,
					event:   event,
					id:      ShortID(),
				}
				logger := l.logger.AddFields(map[string]interface{}{
					force.KeyID: execContext.ID(),
				})
				defer func() {
					err := execContext.Close()
					if err != nil {
						logger.Errorf("Error closing context: %v", err)
					}
				}()
				// add a process logger to the context
				force.SetLog(execContext, logger)
				// add optional data from the event
				event.AddMetadata(execContext)
				start := time.Now()
				err := l.Run.Run(execContext)
				if err != nil {
					logger.WithError(err).Errorf("%v failed after running for %v.", l, time.Now().Sub(start))
				} else {
					logger.Infof("%v completed successfully in %v.", l, time.Now().Sub(start))
				}
			}()
		}
	}
}

// LocalContext implements local execution context
type LocalContext struct {
	*sync.RWMutex
	context context.Context
	process force.Process
	event   force.Event
	id      string
	closers []io.Closer
}

// AddCloser adds closer to the context
func (c *LocalContext) AddCloser(closer io.Closer) {
	c.Lock()
	defer c.Unlock()
	c.closers = append(c.closers, closer)
}

// Close closes the context
// and releases all associated resources
// registered with Closer
func (c *LocalContext) Close() error {
	// this is to prevent possible deadlock on panic
	if c == nil {
		return nil
	}
	c.Lock()
	closers := c.closers
	c.closers = nil
	c.Unlock()
	var errs []error
	for _, c := range closers {
		errs = append(errs, c.Close())
	}
	return trace.NewAggregate(errs...)
}

// ID is an execution unique identifier
func (c *LocalContext) ID() string {
	return c.id
}

// Deadline returns the time when work done on behalf of this context
// should be canceled. Deadline returns ok==false when no deadline is
// set. Successive calls to Deadline return the same results.
func (c *LocalContext) Deadline() (deadline time.Time, ok bool) {
	return c.context.Deadline()
}

func (c *LocalContext) Done() <-chan struct{} {
	return c.context.Done()
}

// Err returns an error associated with the context
// If Done is not yet closed, Err returns nil.
// If Done is closed, Err returns a non-nil error explaining why:
// Canceled if the context was canceled
// or DeadlineExceeded if the context's deadline passed.
// After Err returns a non-nil error, successive calls to Err return the same error.
func (c *LocalContext) Err() error {
	return c.context.Err()
}

// Event is an event that generated the action
func (c *LocalContext) Event() force.Event {
	return c.event
}

// Process returns a process associated with the context
func (c *LocalContext) Process() force.Process {
	return c.process
}

// Value returns a value in the execution context
func (c *LocalContext) Value(val interface{}) interface{} {
	return c.context.Value(val)
}

// SetValue sets a value associated with the keyto the execution context
func (c *LocalContext) SetValue(key interface{}, val interface{}) {
	c.Lock()
	defer c.Unlock()
	c.context = context.WithValue(c.context, key, val)
}
