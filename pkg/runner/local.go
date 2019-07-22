package runner

import (
	"context"
	"fmt"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"
)

// NewLocalProcess starts a new local process
// in the future we may support remote processes,
// e.g. K8s process?
func NewLocalProcess(spec force.Spec) *LocalProcess {
	ctx, cancel := context.WithCancel(context.TODO())
	return &LocalProcess{
		ctx:     ctx,
		cancel:  cancel,
		Spec:    spec,
		eventsC: make(chan force.Event, 32),
	}
}

// LocalProcess implements a process interface
type LocalProcess struct {
	force.Spec
	eventsC chan force.Event
	ctx     context.Context
	cancel  context.CancelFunc
}

func (l *LocalProcess) String() string {
	return fmt.Sprintf("LocalProcess(%v)", l.Name)
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

func (l *LocalProcess) triggerActions(ctx context.Context) {
	for {
		select {
		case <-l.ctx.Done():
			log.Debugf("This process has exited, returning\n")
			return
		case <-ctx.Done():
			log.Debugf("Runner has exited, returning\n")
			return
		case event := <-l.eventsC:
			log.Debugf("%v <- %v", l, event)
			if force.IsExit(event) {
				log.Debugf("Has received an exit event, exiting.")
				log.Debugf("%v has triggered an exit event, exiting.", l)
				l.cancel()
				return
			}
			go func() {
				execContext := &LocalContext{
					context: ctx,
					process: l,
					event:   event,
				}
				_, err := l.Run.Run(execContext)
				if err != nil {
					log.Debugf("%v run completed with %v", l, trace.DebugReport(err))
				} else {
					log.Debugf("%v run completed with success", l)
				}
			}()
		}
	}
}

// LocalContext implements local execution context
type LocalContext struct {
	context context.Context
	process force.Process
	event   force.Event
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
	}
}
