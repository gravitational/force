package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/gravitational/force"
	_ "github.com/gravitational/force/internal/unshare"

	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/sirupsen/logrus"
)

func main() {
	reexec()
	ctx := setupSignalHandlers()
	if err := generateAndStart(ctx); err != nil {
		logrus.Errorf("Error: %v", trace.DebugReport(err))
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

func reexec() {
	// TODO(jessfraz): This is a hack to re-exec our selves and wait for the
	// process since it was not exiting correctly with the constructor.
	if len(os.Getenv("IMG_RUNNING_TESTS")) <= 0 && len(os.Getenv("IMG_DO_UNSHARE")) <= 0 && system.GetParentNSeuid() != 0 {
		var (
			pgid int
			err  error
		)

		// On ^C, or SIGTERM handle exit.
		c := make(chan os.Signal)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			for sig := range c {
				logrus.Infof("Received %s, exiting.", sig.String())
				if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
					logrus.Fatalf("syscall.Kill %d error: %v", pgid, err)
					continue
				}
				os.Exit(0)
			}
		}()

		// If newuidmap is not present re-exec will fail
		if _, err := exec.LookPath("newuidmap"); err != nil {
			logrus.Fatalf("newuidmap not found (install uidmap package?): %v", err)
		}

		// Initialize and re-exec with our unshare.
		cmd := exec.Command("/proc/self/exe", os.Args[1:]...)
		cmd.Env = append(os.Environ(), "IMG_DO_UNSHARE=1")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true,
		}
		if err := cmd.Start(); err != nil {
			logrus.Fatalf("cmd.Start error: %v", err)
		}

		pgid, err = syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			logrus.Fatalf("getpgid error: %v", err)
		}

		var (
			ws       syscall.WaitStatus
			exitCode int
		)
		for {
			// Store the exitCode before calling wait so we get the real one.
			exitCode = ws.ExitStatus()
			_, err := syscall.Wait4(-pgid, &ws, syscall.WNOHANG, nil)
			if err != nil {
				if err.Error() == "no child processes" {
					// We exited. We need to pass the correct error code from
					// the child.
					os.Exit(exitCode)
				}

				logrus.Fatalf("wait4 error: %v", err)
			}
		}
	}
}
