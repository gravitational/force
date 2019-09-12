package force

import (
	"github.com/gravitational/trace"
)

// NewIf creates a new conditional action
// with a new lexical scope
type NewIf struct {
}

// If performs conditional execution of an action
func If(condition BoolVar, action Action) ScopeAction {
	return &IfAction{
		condition: condition,
		action:    action,
	}
}

// NewInstance returns a new instance of a function with a new lexical scope
func (n *NewIf) NewInstance(group Group) (Group, interface{}) {
	return WithLexicalScope(group), If
}

// IfAction runs actions in a sequence,
// if the action fails, next actions are not run
type IfAction struct {
	condition BoolVar
	action    Action
}

// MarshalCode marshals action into code representation
func (p *IfAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   If,
		Args: []interface{}{p.condition, p.action},
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
		return nil
	}
	return s.action.Run(ctx)
}

// Run runs actions in sequence
func (s *IfAction) Run(ctx ExecutionContext) error {
	return s.RunWithScope(WithRuntimeScope(ctx))
}
