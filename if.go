package force

import (
	"github.com/gravitational/trace"
)

// NewIf creates a new conditional action
// with a new lexical scope
type NewIf struct {
}

// If performs conditional execution of an action
func If(condition Expression, action Action, elseAction ...Action) (ScopeAction, error) {
	if err := ExpectBool(condition); err != nil {
		return nil, trace.Wrap(err)
	}
	if len(elseAction) > 1 {
		return nil, trace.BadParameter("only 1 else action is allowed")
	}
	if len(elseAction) == 1 {
		// TODO(klizhentas) make sure function types are compared based on the return value
		// and the signatures
		if err := ExpectEqualTypes(elseAction[0].Type(), action.Type()); err != nil {
			return nil, trace.BadParameter("if and else clauses should evaluate to the same type: %v", err)
		}
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
	condition  Expression
	action     Action
	elseAction Action
}

func (p *IfAction) Type() interface{} {
	return p.action.Type()
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

// EvalWithScope runs actions in sequence using the passed scope
func (s *IfAction) EvalWithScope(ctx ExecutionContext) (interface{}, error) {
	result, err := s.condition.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if !result.(bool) {
		if s.elseAction == nil {
			return false, nil
		}
		return s.elseAction.Eval(ctx)
	}
	return s.action.Eval(ctx)
}

// Eval runs actions in sequence
func (s *IfAction) Eval(ctx ExecutionContext) (interface{}, error) {
	return s.EvalWithScope(WithRuntimeScope(ctx))
}

// NewInstance is used when If expression evaluates to lambda function
func (s *IfAction) NewInstance(group Group) (Group, interface{}) {
	return group, s
}
