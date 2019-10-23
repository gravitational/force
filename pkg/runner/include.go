package runner

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/ioutil"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

// IncludeAction runs shell script
type IncludeAction struct {
	g     *gParser
	paths []force.Expression
}

func (s *IncludeAction) Type() interface{} {
	return []string{}
}

// Eval runs shell script and returns output as a string
func (s *IncludeAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	log := force.Log(ctx)
	var paths []string
	for _, p := range s.paths {
		path, err := force.EvalString(ctx, p)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		paths = append(paths, path)
	}

	for _, path := range paths {
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
		actionI, err := s.g.parseExpr(f, s.g.runner, expr)
		if err != nil {
			return nil, trace.Wrap(convertScanError(err, script))
		}
		if proc, ok := actionI.(force.Process); ok {
			tempContext := force.NewContext(force.ContextConfig{
				Parent:  ctx,
				Process: proc,
				ID:      proc.Name(),
				Event:   &force.OneshotEvent{Time: time.Now().UTC()},
			})
			scopeAction, ok := proc.Action().(force.ScopeAction)
			if !ok {
				return nil, trace.BadParameter("expected scope action, got %T", actionI)
			}
			if _, err := scopeAction.EvalWithScope(tempContext); err != nil {
				return nil, trace.Wrap(err)
			}
		} else {
			action, ok := actionI.(force.ScopeAction)
			if !ok {
				return nil, trace.BadParameter("expected scope action, got %T", actionI)
			}
			_, err = action.EvalWithScope(ctx)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		}
		log.Infof("Included script %v.", path)
	}
	return paths, nil
}

// MarshalCode marshals action into code representation
func (s *IncludeAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Fn: s.g.Include,
	}
	call.Args = make([]interface{}, 0, len(s.paths))
	for i := range s.paths {
		call.Args = append(call.Args, s.paths[i])
	}
	return call.MarshalCode(ctx)
}

func (s *IncludeAction) String() string {
	return fmt.Sprintf("Eval()")
}
