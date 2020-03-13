package runner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/log"

	"github.com/gravitational/trace"
)

// New returns a new instance of the runner
func New(parentCtx context.Context) *Runner {
	ctx, cancel := context.WithCancel(parentCtx)
	globalContext := force.NewContext(force.ContextConfig{
		Parent:  &force.WrapContext{Context: ctx},
		Process: nil,
		ID:      ShortID(),
		Event:   &force.OneshotEvent{Time: time.Now().UTC()},
	})
	return &Runner{
		runners: make(map[string]*Runner),
		cancel:  cancel,
		ctx:     ctx,
		eventsC: make(chan force.Event, 1024),
		plugins: make(map[interface{}]interface{}),
		scope:   force.WithRuntimeScope(globalContext),
	}
}

// Runner listens for events and launches processes
type Runner struct {
	sync.RWMutex
	debugOverride bool
	processes     []force.Process
	channels      []force.Channel
	eventsC       chan force.Event
	cancel        context.CancelFunc
	ctx           context.Context
	runFlag       int32
	exitEvent     force.ExitEvent
	plugins       map[interface{}]interface{}
	logger        force.Logger
	runners       map[string]*Runner
	scope         *force.RuntimeScope
}

// RemoveRunner removes the runner if it matches
// the passed runner pointer (to avoid removing new runner)
func (r *Runner) RemoveRunner(name string, in *Runner) bool {
	r.Lock()
	defer r.Unlock()
	prev, ok := r.runners[name]
	if !ok {
		return false
	}
	if prev != in {
		return false
	}
	delete(r.runners, name)
	return true
}

// SwapRunner sets the runner by name and returns
// the previous version of the runner
func (r *Runner) SwapRunner(name string, in *Runner) *Runner {
	r.Lock()
	defer r.Unlock()
	prev := r.runners[name]
	r.runners[name] = in
	return prev
}

// Logger returns a logger associated with this runner
// if the plugin is set, it will use the plugin to instantiate
// the logger
func (r *Runner) Logger() force.Logger {
	r.Lock()
	defer r.Unlock()
	if r.logger != nil {
		return r.logger
	}
	// if logger is not setup, initialize
	// it from the plugin
	// if the plugin is not set yet, use
	// temporary default one
	pluginI, ok := r.plugins[log.Key]
	if !ok {
		return (&log.Plugin{}).NewLogger()
	}
	plugin, ok := pluginI.(*log.Plugin)
	if !ok {
		temp := (&log.Plugin{}).NewLogger()
		temp.Warningf("Wrong type: %T.", pluginI)
		return temp
	}
	r.logger = plugin.NewLogger()
	return r.logger
}

// IsDebug returns a global debug override
func (r *Runner) IsDebug() bool {
	return r.debugOverride
}

// SetPlugin sets process group-local variable
// all setters and getters are thread safe
func (r *Runner) SetPlugin(key interface{}, val interface{}) {
	r.Lock()
	defer r.Unlock()
	r.plugins[key] = val
}

// GetPlugin returns a process group local variable
// all setters and getters are thread safe
func (r *Runner) GetPlugin(key interface{}) (interface{}, bool) {
	r.RLock()
	defer r.RUnlock()
	val, ok := r.plugins[key]
	return val, ok
}

// Context returns a process group context
func (r *Runner) Context() context.Context {
	return r.ctx
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

func (r *Runner) AddProcess(p force.Process) {
	r.Lock()
	defer r.Unlock()
	r.processes = append(r.processes, p)
	if r.isRunning() {
		p.Start(r.scope)
		go r.wait(p)
	}
}

func (r *Runner) remove(p force.Process) bool {
	r.Lock()
	defer r.Unlock()
	for i := range r.processes {
		if r.processes[i] == p {
			r.processes = append(r.processes[:i], r.processes[i+1:]...)
			return true
		}
	}
	return false
}

func (r *Runner) wait(p force.Process) {
	log := r.Logger()
	log.Debugf("%v waiting for %v", r, p)
	select {
	case <-r.ctx.Done():
		return
	case <-p.Done():
		log.Debugf("%v has exited.", p)
		if !r.remove(p) {
			log.Warningf("%v is not found, can't remove", p)
		}
		return
	}
}

func (r *Runner) ProcessCount() int {
	r.Lock()
	defer r.Unlock()
	return len(r.processes)
}

func (r *Runner) AddChannel(e force.Channel) {
	r.Lock()
	defer r.Unlock()
	r.channels = append(r.channels, e)
	if r.isRunning() {
		go r.fanInEvents(e)
	}
}

// BroadcastEvents will broadcast events
// to every process in a process group
func (r *Runner) BroadcastEvents() chan<- force.Event {
	return r.eventsC
}

func (r *Runner) fanInEvents(channel force.Channel) {
	log := r.Logger()
	log.Debugf("Fan in events: %v", channel)
	if err := channel.Start(context.WithValue(r.ctx, force.KeyLog, log)); err != nil {
		log.Errorf("%v has failed to start: %v", channel, err)
	}

	for {
		select {

		case <-r.Done():
			return
		case <-channel.Done():
			return
		case event := <-channel.Events():
			select {
			case r.eventsC <- event:
				log.Debugf("Fan in received event %v.", event)
			case <-r.Done():
				return
			default:
				log.Warningf("Overflow, dropping event %v", event)
			}
		}
	}
}

func (r *Runner) setExitEvent(event force.ExitEvent) {
	r.Lock()
	defer r.Unlock()
	r.exitEvent = event
}

func (r *Runner) ExitEvent() force.ExitEvent {
	r.RLock()
	defer r.RUnlock()
	return r.exitEvent
}

func (r *Runner) String() string {
	return fmt.Sprintf("Runner(pid=%v)", os.Getpid())
}

func (r *Runner) fanOutEvents() {
	log := r.Logger()
	var shutdownC <-chan time.Time
	for {
		select {
		case <-shutdownC:
			// time to check for processes running count
			count := r.ProcessCount()
			log.Debugf("Process group is shutting down, running process count: %v", count)
			if count == 0 {
				log.Debugf("Process group shut down successfully.")
				r.Close()
				return
			}
		case <-r.Done():
			// this is necessary to close the runner when external context
			// is closed
			r.Close()
			return
		case event := <-r.eventsC:
			// initiate a graceful shutdown
			if exitEvent, ok := event.(force.ExitEvent); ok {
				r.setExitEvent(exitEvent)
				log.Debugf("Runner got an exit event, gracefully shutting down.")
				shutdownTicker := time.NewTicker(200 * time.Millisecond)
				defer shutdownTicker.Stop()
				// the outermost loop will start ticking indicating shutdown
				shutdownC = shutdownTicker.C
			}
			if !r.sendEvent(event) {
				log.Warningf("Failed to send event, returning\n")
				return
			}
		}
	}
}

func (r *Runner) sendEvent(event force.Event) bool {
	log := r.Logger()
	r.RLock()
	defer r.RUnlock()
	for _, proc := range r.processes {
		select {
		case proc.Events() <- event:
			log.Debugf("%v triggered by %v", proc, event)
		case <-r.Done():
			return false
		default:
			log.Warningf("Overflow, dropping event %v for proc %v", event, proc)
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
	log := r.Logger()
	r.Lock()
	defer r.Unlock()
	if r.isRunning() {
		return
	}
	r.start()
	for i := range r.channels {
		go r.fanInEvents(r.channels[i])
	}
	go r.fanOutEvents()
	for _, p := range r.processes {
		if err := p.Start(r.scope); err != nil {
			log.Errorf("%v has failed to start: %v.", p, err)
		}
		log.Debugf("Started process %v.", p)
		go r.wait(p)
	}
}

func (r *Runner) Close() error {
	r.cancel()
	r.stop()
	return nil
}

// OneshotWithExit creates a oneshot process that wraps actions and exits
func (r *Runner) OneshotWithExit(actions ...force.Action) (force.Process, error) {
	actions = append(actions, force.Defer(force.Exit()))
	return r.Oneshot("", actions...)
}

// Oneshot creates a new oneshot process
func (r *Runner) Oneshot(name string, actions ...force.Action) (force.Process, error) {
	seq, err := force.Sequence(actions...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	spec := force.Spec{
		Name:  name,
		Run:   seq,
		Group: r,
	}
	err = spec.CheckAndSetDefaults()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	logger := r.Logger().AddFields(map[string]interface{}{
		force.KeyProc:   spec.Name,
		trace.Component: spec.Name,
	})
	l, err := NewLocalProcess(r.Context(), logger, spec)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return l, nil
}

// Setup creates a new setup process
func (r *Runner) Setup(actions ...force.Action) (force.Process, error) {
	return r.Oneshot("setup", actions...)
}

// Process creates a local process
func (r *Runner) Process(spec force.Spec) (force.Process, error) {
	if err := spec.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	if spec.Group == nil {
		spec.Group = r
	}
	logger := r.Logger().AddFields(map[string]interface{}{
		force.KeyProc:   spec.Name,
		trace.Component: spec.Name,
	})
	l, err := NewLocalProcess(r.Context(), logger, spec)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return l, nil
}

// NewProcess is a function that creates a new process
type NewProcess struct {
	runner *Runner
}

// NewInstance returns a new instance of the process
func (n *NewProcess) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, n.runner.Process
}

// NewSetupProcess creates new setup process
type NewSetupProcess struct {
	runner *Runner
}

// NewInstance returns a new instance of a function
// that does nothing but grouping methods together
func (n *NewSetupProcess) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, n.runner.Setup
}
