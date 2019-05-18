package force

import (
	"context"
	"fmt"
)

// LocalProcess implements local process
type LocalProcess struct {
	Spec
	eventsC chan Event
}

func (l *LocalProcess) String() string {
	return fmt.Sprintf("LocalProcess(%v)", l.Name)

}

// EventSource returns enent source
func (l *LocalProcess) EventSource() EventSource {
	return l.Watch
}

func (l *LocalProcess) Action() Action {
	return l.Run
}

func (l *LocalProcess) Events() chan<- Event {
	return l.eventsC
}

func (l *LocalProcess) Start(ctx context.Context) error {
	go l.triggerActions(ctx)
	return nil
}

func (l *LocalProcess) triggerActions(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("Process exited, returning\n")
			return
		case event := <-l.eventsC:
			fmt.Printf("   %v <- %v\n", l, event)
			go func() {
				if err := l.Run.Run(ctx); err != nil {
					fmt.Printf("%v run completed with %v\n", l, err)
				} else {
					fmt.Printf("%v run completed with success\n", l)
				}
			}()
		}
	}
}
