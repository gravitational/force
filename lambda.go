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

// RunWithScope runs actions in sequence using the passed scope
func (f *LambdaFunction) RunWithScope(scope ExecutionContext) error {
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
		return trace.BadParameter("can't run function with arguments, where would I get those?")
	}
	call := LambdaFunctionCall{LambdaFunction: *f}
	return call.RunWithScope(scope)
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
	}
	io.WriteString(buf, "}")
	return buf.Bytes(), nil
}

// Run runs the action in the context of the worker,
// could modify the context to add metadata, fields or error
func (f *LambdaFunction) Run(ctx ExecutionContext) error {
	return f.RunWithScope(WithRuntimeScope(ctx))
}

// LambdaFunctionCall represents a call of a lambda function with arguments
type LambdaFunctionCall struct {
	LambdaFunction
	// Args defines arguments the lambda function has been called with,
	// binding the evaluated result of the arguments back to their values
	Arguments []Action
}

// AllStatements returns all statements
func (f *LambdaFunctionCall) AllStatements() []Action {
	all := make([]Action, 0, len(f.Arguments)+len(f.Statements))
	all = append(all, f.Arguments...)
	all = append(all, f.Statements...)
	return all
}

// RunWithScope runs actions in sequence using the passed scope
func (f *LambdaFunctionCall) RunWithScope(scope ExecutionContext) error {
	return Sequence(f.AllStatements()...).RunWithScope(scope)
}

// MarshalCode marshals code
func (f *LambdaFunctionCall) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	buf := &bytes.Buffer{}
	data, err := f.LambdaFunction.MarshalCode(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	buf.Write(data)
	io.WriteString(buf, "(")
	for i, arg := range f.Arguments {
		if i != 0 {
			io.WriteString(buf, ", ")
		}
		data, err := arg.MarshalCode(ctx)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		buf.Write(data)
	}
	io.WriteString(buf, ")")
	return buf.Bytes(), nil
}

// Run runs the action in the context of the worker,
// could modify the context to add metadata, fields or error
func (f *LambdaFunctionCall) Run(ctx ExecutionContext) error {
	return f.RunWithScope(WithRuntimeScope(ctx))
}
