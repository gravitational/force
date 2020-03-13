package force

import (
	"context"
	"fmt"
	"time"

	"github.com/gravitational/trace"
)

// FanIn fans in events from multiple channels
func FanIn(channels ...Channel) (Channel, error) {
	if len(channels) == 0 {
		return nil, trace.BadParameter("FanIn needs at least one channel")
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &FanInChannel{
		ctx:     ctx,
		cancel:  cancel,
		in:      channels,
		eventsC: make(chan Event, 1024),
	}, nil
}

// FanInChannel
type FanInChannel struct {
	in      []Channel
	eventsC chan Event
	cancel  context.CancelFunc
	ctx     context.Context
}

func (d *FanInChannel) String() string {
	return fmt.Sprintf("FanIn(channels=%v)", d.in)
}

// wait wiats for all channels to finish
// and cancels it's parent context when
// all channels are done
func (d *FanInChannel) wait(ctx context.Context) {
	defer d.cancel()
	doneC := make(chan struct{}, len(d.in))
	for i := 0; i < len(d.in); i++ {
		go func(channel Channel) {
			select {
			case <-ctx.Done():
				return
			case <-channel.Done():
				doneC <- struct{}{}
			}
		}(d.in[i])
	}
	for range d.in {
		select {
		case <-doneC:
		case <-ctx.Done():
			return
		}
	}
}

func (d *FanInChannel) fanIn(ctx context.Context, channel Channel) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case event := <-channel.Events():
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case d.eventsC <- event:
			}
		}
	}
}

// Start starts all sub channels
// and launches fan in and wait gorotouines
func (d *FanInChannel) Start(ctx context.Context) error {
	log := Log(ctx)
	go d.wait(ctx)
	for i := 0; i < len(d.in); i++ {
		go func(channel Channel) {
			if err := channel.Start(ctx); err != nil {
				log.WithError(err).Errorf("Failed to start %v.", channel)
			}
		}(d.in[i])
		go d.fanIn(ctx, d.in[i])
	}
	return nil
}

func (d *FanInChannel) Done() <-chan struct{} {
	return d.ctx.Done()
}

func (d *FanInChannel) Events() <-chan Event {
	return d.eventsC
}

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

func (e OneshotEvent) AddMetadata(ctx ExecutionContext) {
}

// Ticker returns a channel that fires with period
func Ticker(period string) (Channel, error) {
	if period == "" {
		return nil, trace.BadParameter(
			`set duration parameter, for example Ticker("100s"), supported abbreviations: s (seconds), m (minutes), h (hours), d (days), for example "100m" is tick every 100 minutes`)
	}
	duration, err := time.ParseDuration(string(period))
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return &TickerChannel{
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan Event, 1024),
		period:  duration,
	}, nil
}

type TickerChannel struct {
	eventsC chan Event
	period  time.Duration
}

func (o *TickerChannel) String() string {
	return fmt.Sprintf("Ticker()")
}

func (o *TickerChannel) Start(pctx context.Context) error {
	go func() {
		ticker := time.NewTicker(o.period)
		defer ticker.Stop()
		for {
			select {
			case <-pctx.Done():
				return
			case tm := <-ticker.C:
				select {
				case <-pctx.Done():
					return
				case o.eventsC <- &TickEvent{Time: tm.UTC()}:
				}
			}
		}
	}()
	return nil
}

func (o *TickerChannel) Events() <-chan Event {
	return o.eventsC
}

func (o *TickerChannel) Done() <-chan struct{} {
	return nil
}

type TickEvent struct {
	time.Time
}

func (o *TickEvent) Created() time.Time {
	return o.Time
}

func (o *TickEvent) String() string {
	return fmt.Sprintf("Tick(time=%v)", o.Time)
}

func (e TickEvent) AddMetadata(ctx ExecutionContext) {
}
