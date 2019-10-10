package force

import (
	"github.com/gravitational/trace"
)

// NewIf creates a new conditional action
// with a new lexical scope
type NewIf struct {
}

// If performs conditional execution of an action
func If(condition BoolVar, action Action, elseAction ...Action) (ScopeAction, error) {
	if len(elseAction) > 1 {
		return nil, trace.BadParameter("only 1 else action is allowed")
	}
	return &IfAction{
		condition:  condition,
		action:     action,
		elseAction: elseAction[0],
	}, nil
}

// NewInstance returns a new instance of a function with a new lexical scope
func (n *NewIf) NewInstance(group Group) (Group, interface{}) {
	return WithLexicalScope(group), If
}

// IfAction runs actions in a sequence,
// if the action fails, next actions are not run
type IfAction struct {
	condition  BoolVar
	action     Action
	elseAction Action
}

// MarshalCode marshals action into code representation
func (p *IfAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   If,
		Args: []interface{}{p.condition, p.action},
	}
	if p.elseAction != nil {
		call.Args = append(call.Args, p.elseAction)
	}
	return call.MarshalCode(ctx)
}

// RunWithScope runs actions in sequence using the passed scope
func (s *IfAction) RunWithScope(ctx ExecutionContext) error {
	result, err := s.condition.Eval(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	if !result {
		if s.elseAction == nil {
			return nil
		}
		return s.elseAction.Run(ctx)
	}
	return s.action.Run(ctx)
}

// Run runs actions in sequence
func (s *IfAction) Run(ctx ExecutionContext) error {
	return s.RunWithScope(WithRuntimeScope(ctx))
}
