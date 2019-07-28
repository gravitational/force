package force

import (
	"context"
	"fmt"
	"time"
)

// Oneshot returns a channel that fires once
func Oneshot() (Channel, error) {
	return &OneshotChannel{
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan Event, 1024),
	}, nil
}

type OneshotChannel struct {
	eventsC chan Event
}

func (o *OneshotChannel) String() string {
	return fmt.Sprintf("Oneshot()")
}

func (o *OneshotChannel) Start(pctx context.Context) error {
	go func() {
		select {
		case <-pctx.Done():
			return
		case o.eventsC <- &OneshotEvent{Time: time.Now().UTC()}:
			return
		}
	}()
	return nil
}

func (o *OneshotChannel) Events() <-chan Event {
	return o.eventsC
}

func (o *OneshotChannel) Done() <-chan struct{} {
	return nil
}

type OneshotEvent struct {
	time.Time
}

func (o *OneshotEvent) String() string {
	return fmt.Sprintf("Oneshot(time=%v)", o.Time)
}

func (e OneshotEvent) Wrap(ctx ExecutionContext) ExecutionContext {
	return ctx
}
