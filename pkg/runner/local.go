package runner

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
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

func (l *LocalProcess) Type() interface{} {
	return 0
}

func (l *LocalProcess) fanInEvents(ctx force.ExecutionContext) {
	log := force.Log(ctx)
	for {
		select {
		case <-l.Done():
			return
		case <-l.Channel().Done():
			return
		case <-ctx.Done():
			return
		case event := <-l.Channel().Events():
			select {
			case l.eventsC <- event:
				log.Debugf("Fan in received event %v.", event)
			case <-l.Done():
				return
			case <-ctx.Done():
				return
			default:
				log.Warningf("Overflow, dropping event %v.", event)
			}
		}
	}
}

func (l *LocalProcess) Eval(ctx force.ExecutionContext) (interface{}, error) {
	if err := l.Channel().Start(ctx); err != nil {
		return nil, trace.Wrap(err)
	}
	go l.fanInEvents(ctx)
	l.triggerActions(ctx)
	return 0, nil
}

func (l *LocalProcess) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{FnName: "Process", Args: []interface{}{l.Spec}}
	return call.MarshalCode(ctx)
}

// Done returns a channel that signals that process has completed
// handling channel events and exited
func (l *LocalProcess) Done() <-chan struct{} {
	return l.ctx.Done()
}

func (l *LocalProcess) Events() chan<- force.Event {
	return l.eventsC
}

func (l *LocalProcess) Start(ctx force.ExecutionContext) error {
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

func (l *LocalProcess) triggerActions(ctx force.ExecutionContext) {
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
				execContext := force.NewContext(force.ContextConfig{
					Parent:  ctx,
					Process: l,
					Event:   event,
					ID:      ShortID(),
				})
				logger := l.logger.AddFields(map[string]interface{}{
					force.KeyID: execContext.ID(),
				})
				// add a process logger to the context
				force.SetLog(execContext, logger)
				// add optional data from the event
				event.AddMetadata(execContext)
				start := time.Now()
				lambda, ok := l.Run.(*force.LambdaFunction)
				var err error
				if ok {
					// normally, lambda function evaluates to itself,
					// in Process{Run: func(){}) has to be explicitly callled,
					//
					_, err = lambda.Call(execContext)
				} else {
					_, err = l.Run.Eval(execContext)
				}
				if err != nil {
					logger.WithError(err).Errorf("%v failed after running for %v.", l, time.Now().Sub(start))
				} else {
					logger.Debugf("%v completed successfully in %v.", l, time.Now().Sub(start))
				}
			}()
		}
	}
}
