package force

import (
	"fmt"
)

// NopAction does nothing, is used to wrap arbitrary arguments
type NopAction struct {
	FnName   string
	Args     []Expression
	EvalType interface{}
}

func (s *NopAction) Type() interface{} {
	return s.EvalType
}

// Eval runs shell script and returns output as a string
func (s *NopAction) Eval(ctx ExecutionContext) (interface{}, error) {
	return "nop", nil
}

// MarshalCode marshals action into code representation
func (s *NopAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		FnName: s.FnName,
	}
	call.SetExpressionArgs(s.Args)
	return call.MarshalCode(ctx)
}

func (s *NopAction) String() string {
	return fmt.Sprintf("Nop()")
}
