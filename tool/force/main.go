package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gravitational/force/pkg/runner"

	_ "github.com/gravitational/force/internal/unshare"
	"github.com/gravitational/trace"
	"github.com/opencontainers/runc/libcontainer/system"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	reexec()
	rand.Seed(time.Now().UnixNano())
	ctx := setupSignalHandlers()
	app := kingpin.New("force", "Force is simple CI/CD tool")
	debug := app.Flag("debug", "Turn on debugging level").Short('d').Bool()
	setupFile := app.Flag("setup", "Path to setup file").Default(SetupForce).String()
	forceFile := app.Arg("file", "Force file to run").Default(GFile).String()
	_, err := app.Parse(os.Args[1:])
	if err != nil {
		fmt.Printf("ERROR: %v", err)
		os.Exit(1)
	}

	if err := initLogger(*debug); err != nil {
		fmt.Printf("Failed to init logger: %v", err)
		os.Exit(1)
	}

	run, err := generateAndStart(ctx, *debug, *setupFile, *forceFile)
	if err != nil {
		log.WithError(err).Errorf("Force exited.")
		os.Exit(1)
	}
	select {
	case <-ctx.Done():
		return
	case <-run.Done():
		event := run.ExitEvent()
		if event == nil {
			log.Debugf("Process group has shut down with unkown status.")
		} else {
			log.Debugf("Process group has shut down with event: %v.", event)
			os.Exit(event.ExitCode())
		}
	}
}

// GFile is a special file defining process
const (
	GFile = "G"
	// SetupForce is a special file
	// with setup for the properties
	SetupForce = "setup.force"
)

func generateAndStart(ctx context.Context, debug bool, setupFile, forceFile string) (*runner.Runner, error) {
	setupScript, err := ioutil.ReadFile(setupFile)
	if err == nil {
		log.Debugf("Found setup file %q.", SetupForce)
	}
	script, err := ioutil.ReadFile(forceFile)
	if err != nil {
		return nil, trace.ConvertSystemError(err)
	}
	run, err := runner.Parse(runner.Input{
		Context: ctx,
		Setup:   string(setupScript),
		Script:  string(script),
		Debug:   debug,
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	run.Start()
	return run, nil
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

func initLogger(debug bool) error {
	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	log.SetFormatter(&trace.TextFormatter{
		DisableTimestamp: true,
		EnableColors:     trace.IsTerminal(os.Stderr),
	})
	log.SetOutput(os.Stderr)
	return nil
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
				log.Infof("Received %s, exiting.", sig.String())
				if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
					log.Fatalf("syscall.Kill %d error: %v", pgid, err)
					continue
				}
				os.Exit(0)
			}
		}()

		// If newuidmap is not present re-exec will fail
		if _, err := exec.LookPath("newuidmap"); err != nil {
			log.Fatalf("newuidmap not found (install uidmap package?): %v", err)
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
			log.Fatalf("cmd.Start error: %v", err)
		}

		pgid, err = syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			log.Fatalf("getpgid error: %v", err)
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

				log.Fatalf("wait4 error: %v", err)
			}
		}
	}
}
