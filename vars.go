package force

import (
	"reflect"

	"github.com/gravitational/trace"
)

// NewVarRef returns new variable references
type NewVarRef struct {
}

// NewInstance returns a new instance
func (n *NewVarRef) NewInstance(group Group) (Group, interface{}) {
	return group, Var(group)
}

// Var returns a new function that references a variable
// based on the defined type
func Var(group Group) func(name String, fields ...String) (interface{}, error) {
	return func(name String, fields ...String) (interface{}, error) {
		val, err := group.GetDefinition(string(name))
		if err != nil {
			return nil, trace.Wrap(err)
		}

		v, err := GetField(val, name, fields...)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		e, ok := v.(Expression)
		if ok {
			return &VarRef{name: name, fields: fields, varType: e.Type()}, nil
		}
		return &VarRef{name: name, fields: fields, varType: reflect.TypeOf(v)}, nil
	}
}

// GetField returns field from struct v or error
func GetField(v interface{}, name String, fields ...String) (interface{}, error) {
	if v == nil {
		return nil, trace.BadParameter("%v is not set, does not have field %v", v, name)
	}
	if len(fields) == 0 {
		return v, nil
	}
	vType := reflect.TypeOf(v)
	if vType.Kind() != reflect.Struct {
		return nil, trace.BadParameter("%v has to be struct to have field %v", name, fields[0])
	}
	fieldName := fields[0]
	_, found := vType.FieldByName(string(fieldName))
	if !found {
		return nil, trace.BadParameter("%v does not have a field %v in %#v", name, fieldName, v)
	}
	vVal := reflect.ValueOf(v)
	field := vVal.FieldByName(string(fieldName))
	if !field.IsValid() {
		return nil, trace.BadParameter("%v does not have a field %v in %#v", name, fieldName, v)
	}
	return GetField(field.Interface(), fieldName, fields[1:]...)
}

// VarRef is a variable reference, evaluates to the expression
type VarRef struct {
	name    String
	fields  []String
	varType interface{}
}

func (v *VarRef) Type() interface{} {
	return v.varType
}

// Eval evaluates variable reference and checks types
func (v *VarRef) Eval(ctx ExecutionContext) (interface{}, error) {
	i := ctx.Value(ContextKey(v.name))
	if i == nil {
		return "", trace.BadParameter("variable %v is not set", v.name)
	}
	f, err := GetField(i, v.name, v.fields...)
	if err != nil {
		return "", trace.Wrap(err)
	}
	switch expr := f.(type) {
	case Expression:
		if err := ExpectEqualTypes(expr.Type(), v.varType); err != nil {
			return "", trace.BadParameter("failed to convert variable reference %q of type %v to %v: %v", v.name, v.varType, expr.Type(), err)
		}
		return expr.Eval(ctx)
	default:
		if err := ExpectEqualTypes(expr, v.varType); err != nil {
			return "", trace.BadParameter("failed to convert variable reference %q of type %v to %v: %v", v.name, v.varType, expr, err)
		}
		return expr, nil
	}
}

// MarshalCode evaluates bool variable reference to code representation
func (v *VarRef) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Var, v.name).MarshalCode(ctx)
}

// NewDefine specifies a new define action
type NewDefine struct {
}

// NewInstance returns a new instance of define
func (n *NewDefine) NewInstance(group Group) (Group, interface{}) {
	return group, Define(group)
}

// Define defines a variable type and returns an action
// that sets the variable on the execution
func Define(group Group) func(name String, value interface{}) (Action, error) {
	return func(name String, value interface{}) (Action, error) {
		if err := group.AddDefinition(string(name), value); err != nil {
			return nil, trace.Wrap(err)
		}
		return &DefineAction{
			name:  string(name),
			value: value,
		}, nil
	}
}

// DefineAction defines a variable
type DefineAction struct {
	name  string
	value interface{}
}

// Type returns type evaluated by expression
func (p *DefineAction) Type() interface{} {
	return ExpressionType(p.value)
}

// ExpressionType returns a type evaluated by expression
func ExpressionType(in interface{}) interface{} {
	e, ok := in.(Expression)
	if ok {
		return e.Type()
	}
	return in
}

// Eval defines a variable on the context and evaluates to the value
func (p *DefineAction) Eval(ctx ExecutionContext) (interface{}, error) {
	val, err := Eval(ctx, p.value)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	ctx.SetValue(ContextKey(p.name), val)
	return val, nil
}

// MarshalCode marshals action into code representation
func (p *DefineAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Define,
		Args: []interface{}{p.name, p.value},
	}
	return call.MarshalCode(ctx)
}

// ExpectBool returns nil if expression is bool, error otherwise
func ExpectBool(expr Expression) error {
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf(false)) {
		return nil
	}
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf(BoolVar{})) {
		return nil
	}
	return trace.BadParameter("%v does not evaluate to bool", expr.Type())
}

// ExpectLambdaFunction expects expression to evaluate to lambda function
func ExpectLambdaFunction(expr Expression) (*LambdaFunction, error) {
	iface := expr.Type()
eval:
	switch eType := iface.(type) {
	case *LambdaFunction:
		return eType, nil
	case Expression:
		iface = eType.Type()
		goto eval
	default:
		return nil, trace.BadParameter("expected lambda function, got %T", iface)
	}
}

// ExpectString returns nil if expression is bool, error otherwise
func ExpectString(expr Expression) error {
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf("")) {
		return nil
	}
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf(StringVar{})) {
		return nil
	}
	return trace.BadParameter("%v does not evaluate to string", expr.Type())
}

// ExpectInt returns nil if expression is int, error otherwise
func ExpectInt(expr Expression) error {
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf(0)) {
		return nil
	}
	if reflect.TypeOf(expr.Type()).AssignableTo(reflect.TypeOf(IntVar{})) {
		return nil
	}
	return trace.BadParameter("%v does not evaluate to int", expr.Type())
}

// ExpectEqualTypes compares expression types
func ExpectEqualTypes(aType, bType interface{}) error {
	if aType == nil || bType == nil {
		return trace.BadParameter("can't compare nil types")
	}
	if expr, ok := aType.(Expression); ok {
		aType = expr.Type()
	}
	if expr, ok := bType.(Expression); ok {
		bType = expr.Type()
	}
	checker, ok := aType.(TypeChecker)
	if ok {
		return checker.ExpectEqualTypes(bType)
	}
	if reflect.TypeOf(aType) != reflect.TypeOf(bType) {
		return trace.BadParameter("%v type does not match %v", aType, bType)
	}
	return nil
}
