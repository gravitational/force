package force

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	log "github.com/sirupsen/logrus"
)

// Runner listens for events and launches processes
type Runner struct {
	sync.RWMutex
	processes    []Proc
	eventSources []EventSource
	eventsC      chan Event
	cancel       context.CancelFunc
	ctx          context.Context
	runFlag      int32
}

func (r *Runner) isRunning() bool {
	return atomic.LoadInt32(&r.runFlag) == 1
}

func (r *Runner) stop() {
	atomic.StoreInt32(&r.runFlag, 0)
}

func (r *Runner) start() {
	atomic.StoreInt32(&r.runFlag, 1)
}

func (r *Runner) AddProcess(p Proc) {
	r.Lock()
	defer r.Unlock()
	r.processes = append(r.processes, p)
	if r.isRunning() {
		p.Start(r.ctx)
	}
}

func (r *Runner) AddEventSource(e EventSource) {
	r.Lock()
	defer r.Unlock()
	r.eventSources = append(r.eventSources, e)
	if r.isRunning() {
		go r.fanInEvents(e)
	}
}

func (r *Runner) fanInEvents(source EventSource) {
	fmt.Printf("Fan in events: %v\n", source)
	if err := source.Start(r.ctx); err != nil {
		fmt.Printf("%v has failed to start: %v\n", source, err)
	}
	for {
		select {
		case <-r.Done():
			return
		case <-source.Done():
			return
		case event := <-source.Events():
			select {
			case r.eventsC <- event:
				fmt.Printf("<- %v\n", event)
			case <-r.Done():
				return
			default:
				log.Warningf("Overflow, dropping event %v", event)
			}
		}
	}
}

func (r *Runner) fanOutEvents() {
	for {
		select {
		case <-r.Done():
			// this is necessary to close the runner when external context
			// is closed
			r.Close()
			return
		case event := <-r.eventsC:
			if !r.sendEvent(event) {
				fmt.Printf("Failed to send event, returning\n")
				return
			}
		}
	}
}

func (r *Runner) sendEvent(event Event) bool {
	r.RLock()
	defer r.RUnlock()
	for _, proc := range r.processes {
		select {
		case proc.Events() <- event:
			fmt.Printf("  %v <- %v\n", proc, event)

		case <-r.Done():
			return false
		default:

			log.Warningf("Overflow, dropping event %v for proc", event, proc)
		}
	}
	return true
}

// Done returns channel
func (r *Runner) Done() <-chan struct{} {
	return r.ctx.Done()
}

// Start is a non blocking call
func (r *Runner) Start() {
	r.Lock()
	defer r.Unlock()
	if r.isRunning() {
		return
	}
	r.start()
	for i := range r.eventSources {
		go r.fanInEvents(r.eventSources[i])
	}
	go r.fanOutEvents()
	for _, p := range r.processes {
		if err := p.Start(r.ctx); err != nil {
			fmt.Printf("%v has failed to start: %v\n", p, err)
		}
	}
}

func (r *Runner) Close() error {
	r.cancel()
	r.stop()
	return nil
}

// Process creates a local process
func (r *Runner) Process(spec Spec) Proc {
	l := &LocalProcess{
		Spec:    spec,
		eventsC: make(chan Event, 32),
	}
	r.AddProcess(l)
	// TODO: how to deduplicate event sources?
	if spec.Watch != nil {
		fmt.Printf("Add event source %v\n", spec.Watch)
		r.AddEventSource(spec.Watch)
	}
	return l
}

func NewRunner(ctx context.Context) *Runner {
	ctx, cancel := context.WithCancel(ctx)
	return &Runner{
		cancel:  cancel,
		ctx:     ctx,
		eventsC: make(chan Event, 1024),
	}
}

var runner *Runner

func init() {
	runner = NewRunner(context.TODO())
}

// GLobalRunner is not such a great idea,
// think about alternatives
func GlobalRunner() *Runner {
	return runner
}
