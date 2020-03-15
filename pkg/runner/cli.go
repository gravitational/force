package runner

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/gravitational/force"
	forcelog "github.com/gravitational/force/pkg/log"
	"github.com/gravitational/trace"
	log "github.com/sirupsen/logrus"

	"github.com/opencontainers/runc/libcontainer/system"
	"gopkg.in/alecthomas/kingpin.v2"
)

// ExitWithoutEnv expects environment variable, exits with error
// if the environment variable is missing
func ExitWithoutEnv(key string) string {
	val, err := force.ExpectEnv(key)
	ExitIf(err)
	return val
}

// Run is a shortcut to Setup().Run()
func Run(fn force.Action) {
	Setup().Run(fn)
}

// RunFunc is a shortcut to Setup().Run(force.ActionFunc(fn))
func RunFunc(fn force.ActionFunc) {
	Setup().Run(force.ActionFunc(fn))
}

// Setup sets up force plugins and creates a command line tool
func Setup(setupFns ...force.SetupFunc) *CLIRunner {
	app := kingpin.New("force", "Force is simple CI/CD tool")
	var debug bool
	app.Flag("debug", "Turn on debugging level").Short('d').BoolVar(&debug)

	_, err := app.Parse(os.Args[1:])
	ExitIf(err)
	InitLogger(debug)
	rand.Seed(time.Now().UnixNano())
	ctx := SetupSignalHandlers()
	return SetupInCLI(ctx, setupFns...)
}

// SetupInCLI sets up force plugins assuming CLI was already set up
func SetupInCLI(ctx context.Context, setupFns ...force.SetupFunc) *CLIRunner {
	rand.Seed(time.Now().UnixNano())

	runner := New(ctx)
	for _, setupFn := range setupFns {
		if err := setupFn(runner); err != nil {
			ExitIf(err)
		}
	}
	return &CLIRunner{Runner: runner}
}

// CLIRunner is a command line wrapper
// with some CLI helper functions
type CLIRunner struct {
	*Runner
	name    string
	channel force.Channel
}

// Name sets process name
func (runner *CLIRunner) Name(name string) *CLIRunner {
	runner.name = name
	return runner
}

// Watch sets up process watcher
func (runner *CLIRunner) Watch(newChannel force.NewChannelFunc) *CLIRunner {
	channel, err := newChannel(runner)
	ExitIf(err)
	runner.channel = channel
	return runner
}

// RunFunc is like run but for function
func (runner *CLIRunner) RunFunc(fn force.ActionFunc) {
	runner.Run(fn)
}

// Run creates the process and runs it to completion
func (runner *CLIRunner) Run(fn force.Action) {
	var proc force.Process
	if runner.channel == nil {
		var err error
		proc, err = runner.OneshotWithExit(fn)
		ExitIf(err)
	} else {
		var err error
		proc, err = runner.Runner.Process(force.Spec{
			Name:  runner.name,
			Watch: runner.channel,
			Run:   fn,
		})
		ExitIf(err)
	}

	runner.runProcess(proc)
}

func (runner *CLIRunner) runProcess(proc force.Process) {
	runner.AddProcess(proc)
	runner.AddChannel(proc.Channel())
	runner.Start()
	select {
	case <-runner.Done():
		event := runner.ExitEvent()
		if event == nil {
			log.Debugf("Process group has shut down with unknown status.")
		} else {
			log.Debugf("Process group has shut down with event: %v.", event)
			os.Exit(event.ExitCode())
		}
	}
}

// ExitIf prints error and exits the process
// does nothing if error is nil
func ExitIf(err error) {
	if err == nil {
		return
	}
	if trace.IsDebug() {
		fmt.Fprintln(os.Stderr, trace.DebugReport(err))
	} else {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	os.Exit(1)
}

// SetupSignalHandlers sets up a handler to handle common unix process signal traps.
// Some signals are handled to avoid the default handling which might be termination (SIGPIPE, SIGHUP, etc)
// The rest are considered as termination signals and the handler initiates shutdown upon receiving
// such a signal.
func SetupSignalHandlers() context.Context {
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

func InitLogger(debug bool) error {
	if debug {
		trace.SetDebug(true)
		log.SetLevel(log.DebugLevel)
		log.SetFormatter(&trace.TextFormatter{
			DisableTimestamp: true,
			EnableColors:     trace.IsTerminal(os.Stderr),
		})
	} else {
		log.SetLevel(log.InfoLevel)
		log.SetFormatter(&forcelog.TerminalFormatter{})
	}
	log.SetOutput(os.Stderr)
	return nil
}

func Reexec() {
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
				log.Debugf("Received %s, exiting.", sig.String())
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
