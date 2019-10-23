package runner

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// LoadAction
type LoadAction struct {
	g      *gParser
	path   force.Expression
	reload bool
}

func (s *LoadAction) Type() interface{} {
	return 0
}

// Eval runs shell script and returns output as a string
func (s *LoadAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	log := force.Log(ctx)

	path, err := force.EvalString(ctx, s.path)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	content, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	script := Script{
		Filename: path,
		Content:  string(content),
	}
	f := token.NewFileSet()
	expr, err := parser.ParseExprFrom(f, "", content, 0)
	if err != nil {
		return nil, trace.Wrap(convertScanError(err, script))
	}

	runnerCtx, cancel := context.WithCancel(ctx)
	runner := &Runner{
		LexScope:      force.WithLexicalScope(nil),
		parser:        s.g,
		debugOverride: s.g.runner.debugOverride,
		cancel:        cancel,
		ctx:           runnerCtx,
		eventsC:       make(chan force.Event, cap(s.g.runner.eventsC)),
		plugins:       make(map[interface{}]interface{}),
		logger:        s.g.runner.Logger(),
	}
	actionI, err := s.g.parseExpr(f, runner, expr)
	if err != nil {
		return nil, trace.Wrap(convertScanError(err, script))
	}
	proc, ok := actionI.(force.Process)
	if !ok {
		action, ok := actionI.(force.Action)
		if !ok {
			defer runner.Close()
			return nil, trace.BadParameter("expected action, got %T", actionI)
		}
		var err error
		proc, err = runner.Oneshot(force.KeyForce, action)
		if err != nil {
			defer runner.Close()
			return nil, trace.Wrap(err)
		}
	}
	// reload tracks all the runners for processes by name,
	// and stops the previous version if necessary
	if s.reload {
		defer s.g.runner.RemoveRunner(proc.Name(), runner)
		prevRunner := s.g.runner.SwapRunner(proc.Name(), runner)
		if prevRunner != nil {
			prevRunner.Close()
			select {
			case <-prevRunner.Done():
				log.Infof("Reload: previous runner for the process %v has completed.", proc)
				event := runner.ExitEvent()
				if event == nil {
					log.Debugf("Process group has shut down with unkown status.")
				} else {
					log.Debugf("Process group has shut down with event: %v.", event)
				}
			case <-ctx.Done():
				defer runner.Close()
				return nil, trace.ConnectionProblem(ctx.Err(), "parent context is closing")
			}
		}
	}
	log.Infof("Started subprocess %v.", proc.Name())
	runner.AddChannel(proc.Channel())
	runner.AddProcess(proc)
	runner.Start()
	select {
	case <-runner.Done():
		log.Infof("Runner for process %v is done.", proc)
		event := runner.ExitEvent()
		if event == nil {
			log.Debugf("Process group has shut down with unkown status.")
			return 0, nil
		}
		log.Debugf("Process group has shut down with event: %v.", event)
		if event.ExitCode() != 0 {
			return event.ExitCode(), trace.BadParameter("runner failed with exit code %v", event.ExitCode())
		}
		return 0, nil
	case <-ctx.Done():
		return nil, trace.ConnectionProblem(ctx.Err(), "parent context is closing")
	}
}

// MarshalCode marshals action into code representation
func (s *LoadAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Args: []interface{}{s.path},
	}
	if s.reload {
		call.Fn = s.g.Reload
	} else {
		call.Fn = s.g.Load
	}
	return call.MarshalCode(ctx)
}

func (s *LoadAction) String() string {
	return fmt.Sprintf("Load(reload=%v)", s.reload)
}
