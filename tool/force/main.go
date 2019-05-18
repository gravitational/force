package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

func main() {
	ctx := setupSignalHandlers()
	if err := generateAndStart(ctx); err != nil {
		fmt.Printf("Error: %v", trace.DebugReport(err))
		os.Exit(1)
	}
	<-ctx.Done()
}

// GFile is a special file defining process
const GFile = "G"

func generateAndStart(ctx context.Context) error {
	data, err := ioutil.ReadFile(GFile)
	if err != nil {
		return trace.Wrap(err)
	}
	runner := force.NewRunner(ctx)
	err = force.Parse(string(data), runner)
	if err != nil {
		return trace.Wrap(err)
	}
	runner.Start()
	return nil
}

// setupSignalHandlers sets up a handler to handle common unix process signal traps.
// Some signals are handled to avoid the default handling which might be termination (SIGPIPE, SIGHUP, etc)
// The rest are considered as termination signals and the handler initiates shutdown upon receiving
// such a signal.
func setupSignalHandlers() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	c := make(chan os.Signal, 1)
	go func() {
		defer cancel()
		for sig := range c {
			fmt.Printf("Received a %s signal, exiting...\n", sig)
			return
		}
	}()
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	return ctx
}
