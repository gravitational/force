package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
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
	return l.Spec.Name
}

// String returns process user friendly string
func (l *LocalProcess) String() string {
	return fmt.Sprintf("Process %v", l.Spec.Name)
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
				r, err := uuid.NewRandom()
				if err != nil {
					// Failed to generate random number, very sad.
					panic(err)
				}
				execContext := &LocalContext{
					context: ctx,
					process: l,
					event:   event,
					id:      r.String(),
				}
				logger := l.logger.AddFields(map[string]interface{}{
					force.KeyID: execContext.ID(),
				})
				// add a process logger to the context
				outContext := force.WithLog(execContext, logger)
				// add optional data from the event
				outContext = event.Wrap(outContext)
				_, err = l.Run.Run(outContext)
				if err != nil {
					logger.Errorf("%v failed: %v.", l, fullMessage(err))
				} else {
					logger.Infof("%v completed successfully.", l)
				}
			}()
		}
	}
}

func fullMessage(err error) string {
	if trace.IsDebug() {
		return trace.DebugReport(err)
	}
	userMessage := fmt.Sprintf("%v", trace.Unwrap(err))
	errMessage := fmt.Sprintf(err.Error())
	if errMessage != userMessage {
		return fmt.Sprintf("%v, %v", errMessage, userMessage)
	}
	return userMessage
}

// LocalContext implements local execution context
type LocalContext struct {
	context context.Context
	process force.Process
	event   force.Event
	id      string
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

func (c *LocalContext) Context() context.Context {
	return c.context
}

func (c *LocalContext) Process() force.Process {
	return c.process
}

// Get returns a value in the execution context
func (c *LocalContext) Value(val interface{}) interface{} {
	return c.context.Value(val)
}

// WithValue extends (without modifying) the execution context
// with a single key value pair
func (c *LocalContext) WithValue(key interface{}, val interface{}) force.ExecutionContext {
	return &LocalContext{
		context: context.WithValue(c.context, key, val),
		process: c.process,
		event:   c.event,
		id:      c.id,
	}
}
