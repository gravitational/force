package force

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// Exit exits, if the exit code has been supplied
// it will extract for whatever exit event was sent in the context
func Exit() (Action, error) {
	return &SendAction{
		GetEvent: GetExitEventFromContext,
	}, nil
}

type GetEventFunc func(ctx ExecutionContext) Event

func GetExitEventFromContext(ctx ExecutionContext) Event {
	val := ctx.Context().Value(ExitCode)
	event := &LocalExitEvent{}
	if code, ok := val.(int); ok {
		log.Debugf("Found code: %v.", code)
		event.Code = code
	}
	return event
}

type SendAction struct {
	GetEvent GetEventFunc
	Process  Process
}

func (e *SendAction) Run(ctx ExecutionContext) (ExecutionContext, error) {
	proc := e.Process
	// no process specified? assume broadcast to the process group
	if proc == nil {
		proc = ctx.Process()
	}
	select {
	case proc.Group().BroadcastEvents() <- e.GetEvent(ctx):
		return ctx, nil
	case <-ctx.Context().Done():
		return ctx, ctx.Context().Err()
	}
}

// ExitEvent is a special event
// tells the process group to exit with a specified code
type ExitEvent interface {
	ExitCode() int
}

type LocalExitEvent struct {
	Code int
}

func (e LocalExitEvent) ExitCode() int {
	return e.Code
}

// String returns a string description of the event
func (e LocalExitEvent) String() string {
	return fmt.Sprintf("Exit(code=%v)", e.Code)
}

// IsExit returns true if it's an exit event
func IsExit(event Event) bool {
	_, ok := event.(ExitEvent)
	log.Debugf("Is exit %v -> %v.", event, ok)
	return ok
}
