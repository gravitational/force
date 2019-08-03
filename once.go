package force

import (
	"context"
	"fmt"
	"time"
)

// Duplicate creates a channel that sends the same event
// count times, used for testing purposes
func Duplicate(c Channel, count int) Channel {
	return &DuplicateChannel{
		in:      c,
		eventsC: make(chan Event, 1024),
		count:   count,
	}
}

//
type DuplicateChannel struct {
	in      Channel
	eventsC chan Event
	count   int
}

func (d *DuplicateChannel) String() string {
	return fmt.Sprintf("Duplicate(count=%v)", d.count)
}

func (d *DuplicateChannel) Start(pctx context.Context) error {
	go d.in.Start(pctx)
	go func() {
		for {
			select {
			case <-pctx.Done():
				return
			case <-d.in.Done():
				return
			case event := <-d.in.Events():
				for i := 0; i < d.count; i++ {
					select {
					case <-pctx.Done():
						return
					case <-d.in.Done():
						return
					case d.eventsC <- event:
					}
				}
			}
		}
	}()
	return nil
}

func (d *DuplicateChannel) Done() <-chan struct{} {
	return d.in.Done()
}

func (d *DuplicateChannel) Events() <-chan Event {
	return d.eventsC
}

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

func (o *OneshotEvent) Created() time.Time {
	return o.Time
}

func (o *OneshotEvent) String() string {
	return fmt.Sprintf("Oneshot(time=%v)", o.Time)
}

func (e OneshotEvent) Wrap(ctx ExecutionContext) ExecutionContext {
	return ctx
}
