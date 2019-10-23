package force

import (
	"bytes"
	"fmt"
	"io"
	"reflect"

	"github.com/gravitational/trace"
)

type LambdaParam struct {
	Name      string
	Prototype interface{}
}

type LambdaFunction struct {
	Scope      Group
	Statements []Action
	Params     []LambdaParam
}

func (f *LambdaFunction) NewInstance(group Group) (Group, interface{}) {
	return group, f
}

func (f *LambdaFunction) Type() interface{} {
	return f
}

// NewCall creates a new call
func (f *LambdaFunction) NewCall() (*LambdaFunctionCall, error) {
	// lambda function definition can be run as action,
	// this is done specifically for case:
	//
	// Proc(Spec{Run: func(){})
	//
	// that parses into:
	//
	// Run: LambdaFunction{}
	//
	if len(f.Params) != 0 {
		return nil, trace.BadParameter("can not run, this lambda function should not have arguments")
	}
	return &LambdaFunctionCall{Expression: f}, nil
}

// Call attempts to call a function with no arguments
func (f *LambdaFunction) Call(scope ExecutionContext) (interface{}, error) {
	call, err := f.NewCall()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return call.EvalWithScope(scope)
}

// EvalWithScope runs actions in sequence using the passed scope
func (f *LambdaFunction) EvalWithScope(scope ExecutionContext) (interface{}, error) {
	return f, nil
}

// MarshalCode marshals code
func (f *LambdaFunction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	buf := &bytes.Buffer{}
	io.WriteString(buf, "func(")
	for i, param := range f.Params {
		if i != 0 {
			io.WriteString(buf, ", ")
		}
		fmt.Fprintf(buf, "%v %v", param.Name, reflect.TypeOf(param).Name())
	}
	io.WriteString(buf, "){")
	for _, statement := range f.Statements {
		data, err := statement.MarshalCode(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		buf.Write(data)
		io.WriteString(buf, "\n")
	}
	io.WriteString(buf, "}")
	return buf.Bytes(), nil
}

// Eval runs the action in the context of the worker,
// could modify the context to add metadata, fields or error
func (f *LambdaFunction) Eval(ctx ExecutionContext) (interface{}, error) {
	return f.EvalWithScope(WithRuntimeScope(ctx))
}

// TypeChecker is used to test two types for equivalence
type TypeChecker interface {
	// ExpectEqualTypes returns nil if the types are identical
	// error otherwise
	ExpectEqualTypes(i interface{}) error
}

// ExpectEqualTypes returns nil if the lambda function signatures
// (input argument types and evaluated value) are equal
func (f *LambdaFunction) ExpectEqualTypes(i interface{}) error {
	b, ok := i.(*LambdaFunction)
	if !ok {
		return trace.BadParameter("expected LambdaFunction, got %v", i)
	}
	if len(f.Params) != len(b.Params) {
		return trace.BadParameter("expected %v parameters, other function has %v", len(f.Params), len(b.Params))
	}
	for i := range f.Params {
		paramA := f.Params[i]
		paramB := b.Params[i]
		if err := ExpectEqualTypes(paramA.Prototype, paramB.Prototype); err != nil {
			return trace.BadParameter("mismatch type for function parameters %v and %v: %v", paramA.Name, paramB.Name, err)
		}
	}
	if len(f.Statements) != 0 && len(b.Statements) == 0 {
		return nil
	}
	if len(f.Statements) == 0 || len(b.Statements) == 0 {
		return trace.BadParameter("functions do not evaluate to the same type, one is empty")
	}
	return ExpectEqualTypes(f.Statements[len(f.Statements)-1], b.Statements[len(b.Statements)-1])
}

// LambdaFunctionCall represents a call of a lambda function with arguments
type LambdaFunctionCall struct {
	Expression Expression
	// Args defines arguments the lambda function has been called with
	Arguments []interface{}
}

// CheckCall checks call type variables and parameters
func (f *LambdaFunctionCall) CheckCall() error {
	lambda, err := f.LambdaType()
	if err != nil {
		return trace.Wrap(err)
	}
	if len(f.Arguments) != len(lambda.Params) {
		return trace.BadParameter("expected %v parameters, got %v arguments", len(lambda.Params), len(f.Arguments))
	}
	for i := range f.Arguments {
		arg := f.Arguments[i]
		param := lambda.Params[i]
		if err := ExpectEqualTypes(arg, param.Prototype); err != nil {
			return trace.BadParameter("mismatch type for function parameter %v: %v", param.Name, err)
		}
	}
	return nil
}

// EvalStatements returns all statements from evaluated lambda function
func (f *LambdaFunctionCall) EvalStatements(ctx ExecutionContext) ([]Action, error) {
	iface, err := f.Expression.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	lambda, ok := iface.(*LambdaFunction)
	if !ok {
		return nil, trace.BadParameter("expected LambdaFunction, got %T", iface)
	}
	callScope := WithLexicalScope(lambda.Scope)
	// in case of lambda function, passed arguments
	// are converted into defined statements
	callArgs := make([]Action, len(f.Arguments))
	for i, param := range lambda.Params {
		def, err := Define(callScope)(String(param.Name), f.Arguments[i])
		if err != nil {
			return nil, trace.Wrap(err)
		}
		callArgs[i] = def
	}
	all := make([]Action, 0, len(f.Arguments)+len(lambda.Statements))
	all = append(all, callArgs...)
	all = append(all, lambda.Statements...)
	return all, nil
}

// LambdaType returns a lambda function type
func (f *LambdaFunctionCall) LambdaType() (*LambdaFunction, error) {
	return ExpectLambdaFunction(f.Expression)
}

// LambdaFunctionCall evaluates to it's last statement
func (f *LambdaFunctionCall) Type() interface{} {
	lambda, err := f.LambdaType()
	if err != nil {
		panic(err)
	}
	return lambda.Statements[len(lambda.Statements)-1].Type()
}

// EvalWithScope runs actions in sequence using the passed scope and evalates
// to the value of the sequence (the last element of it)
func (f *LambdaFunctionCall) EvalWithScope(scope ExecutionContext) (interface{}, error) {
	statements, err := f.EvalStatements(scope)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	seq, err := Sequence(statements...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return seq.EvalWithScope(scope)
}

// MarshalCode marshals code
func (f *LambdaFunctionCall) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	buf := &bytes.Buffer{}
	data, err := f.Expression.MarshalCode(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	buf.Write(data)
	io.WriteString(buf, "(")
	for i, arg := range f.Arguments {
		if i != 0 {
			io.WriteString(buf, ", ")
		}
		data, err := MarshalCode(ctx, arg)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		buf.Write(data)
	}
	io.WriteString(buf, ")")
	return buf.Bytes(), nil
}

// Eval runs the action in the context of the worker,
// could modify the context to add metadata, fields or error
func (f *LambdaFunctionCall) Eval(ctx ExecutionContext) (interface{}, error) {
	return f.EvalWithScope(WithRuntimeScope(ctx))
}
