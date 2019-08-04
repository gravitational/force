package force

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gravitational/trace"
)

func Files(files ...string) (Channel, error) {
	if len(files) == 0 {
		return nil, trace.BadParameter("Files() needs at least one file")
	}
	expanded := []string{}
	for _, file := range files {
		matches, err := filepath.Glob(file)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		expanded = append(expanded, matches...)
	}
	return &FSNotify{
		Files: expanded,
		// TODO(klizhentas): queues have to be configurable
		eventsC: make(chan Event, 1024),
	}, nil
}

type FSNotify struct {
	Files   []string
	eventsC chan Event
}

func (f *FSNotify) String() string {
	return fmt.Sprintf("Files(%v)", strings.Join(f.Files, ","))
}

func (f *FSNotify) Start(pctx context.Context) error {
	log := Log(pctx)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return trace.Wrap(err)
	}
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-pctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Remove == fsnotify.Remove {
					fevent := &FSNotifyEvent{Event: event, created: time.Now().UTC()}
					select {
					case f.eventsC <- fevent:
						log.Debugf("Sending %v.", fevent)
					case <-pctx.Done():

						return
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf("FS Notify error:%v\n", err)
			}
		}
	}()

	for _, file := range f.Files {
		err = watcher.Add(file)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
	}
	return nil
}

func (f *FSNotify) Events() <-chan Event {
	return f.eventsC
}

func (f *FSNotify) Done() <-chan struct{} {
	return nil
}

type FSNotifyEvent struct {
	fsnotify.Event
	created time.Time
}

func (f *FSNotifyEvent) Created() time.Time {
	return f.created
}

func (f *FSNotifyEvent) String() string {
	return fmt.Sprintf("File(name=%v, action=%v)", f.Event.Name, f.Event.Op.String())
}

func (f *FSNotifyEvent) AddMetadata(ctx ExecutionContext) {
}
