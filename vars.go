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

		switch v.(type) {
		case StringVar:
			return &StringVarRef{name: name, fields: fields}, nil
		case String:
			return &StringVarRef{name: name, fields: fields}, nil
		case IntVar:
			return &IntVarRef{name: name, fields: fields}, nil
		case Int:
			return &IntVarRef{name: name, fields: fields}, nil
		case BoolVar:
			return &BoolVarRef{name: name, fields: fields}, nil
		case Bool:
			return &BoolVarRef{name: name, fields: fields}, nil
		case []String:
			return &StringsVarRef{name: name, fields: fields}, nil
		case []StringVar:
			return &StringsVarRef{name: name, fields: fields}, nil
		case StringVarSlice:
			return &StringsVarRef{name: name, fields: fields}, nil
		}
		return nil, trace.BadParameter("unsupported reference type %T", v)
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

// StringsVarRef is a string variable reference
type StringsVarRef struct {
	name   String
	fields []String
}

// Eval evaluates string var reference
func (s *StringsVarRef) Eval(ctx ExecutionContext) ([]string, error) {
	i := ctx.Value(ContextKey(s.name))
	if i == nil {
		return nil, trace.BadParameter("variable %v is not set", s.name)
	}
	f, err := GetField(i, s.name, s.fields...)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	switch val := f.(type) {
	case []string:
		return val, nil
	case []String:
		out := make([]string, len(val))
		for i, s := range val {
			out[i] = string(s)
		}
		return out, nil
	case []StringVar:
		return EvalStringVars(ctx, val)
	case StringsVar:
		return val.Eval(ctx)
	}
	return nil, trace.BadParameter("failed to convert variable %v to []string with type %T", s.name, i)
}

// MarshalCode evaluates bool variable reference to code representation
func (s *StringsVarRef) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Var, s.name).MarshalCode(ctx)
}

// StringVarRef is a string variable reference
type StringVarRef struct {
	name   String
	fields []String
}

// Eval evaluates string var reference
func (s *StringVarRef) Eval(ctx ExecutionContext) (string, error) {
	i := ctx.Value(ContextKey(s.name))
	if i == nil {
		return "", trace.BadParameter("variable %v is not set", s.name)
	}
	f, err := GetField(i, s.name, s.fields...)
	if err != nil {
		return "", trace.Wrap(err)
	}
	switch val := f.(type) {
	case string:
		return val, nil
	case StringVar:
		return val.Eval(ctx)
	}
	return "", trace.BadParameter("failed to convert variable %q of type %T to string", s.name, f)
}

// MarshalCode evaluates bool variable reference to code representation
func (s *StringVarRef) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Var, s.name).MarshalCode(ctx)
}

// IntVarRef is a new integer variable reference
type IntVarRef struct {
	name   String
	fields []String
}

// Eval evalutates int variable
func (i *IntVarRef) Eval(ctx ExecutionContext) (int, error) {
	iface := ctx.Value(ContextKey(i.name))
	if iface == nil {
		return -1, trace.BadParameter("variable %v is not set", i.name)
	}
	f, err := GetField(iface, i.name, i.fields...)
	if err != nil {
		return -1, trace.Wrap(err)
	}
	switch val := f.(type) {
	case int:
		return val, nil
	case Int:
		return int(val), nil
	case IntVar:
		return val.Eval(ctx)
	}
	return -1, trace.BadParameter("failed to convert variable %q to int", i.name)
}

// MarshalCode evaluates bool variable reference to code representation
func (i *IntVarRef) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Var, i.name).MarshalCode(ctx)
}

// BoolVarRef is a bool variable reference
type BoolVarRef struct {
	name   String
	fields []String
}

// Eval evaluates reference to it's bool variable
func (b *BoolVarRef) Eval(ctx ExecutionContext) (bool, error) {
	i := ctx.Value(ContextKey(b.name))
	if i == nil {
		return false, trace.BadParameter("variable %v is not set", b.name)
	}
	f, err := GetField(i, b.name, b.fields...)
	if err != nil {
		return false, trace.Wrap(err)
	}
	switch val := f.(type) {
	case bool:
		return val, nil
	case BoolVar:
		return val.Eval(ctx)
	}
	return false, trace.BadParameter("failed to convert variable %q to bool", b.name)
}

// MarshalCode evaluates bool variable reference to code representation
func (b *BoolVarRef) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return NewFnCall(Var, b.name).MarshalCode(ctx)
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

// Run defines a variable on the context
func (p *DefineAction) Run(ctx ExecutionContext) error {
	val, err := Eval(ctx, p.value)
	if err != nil {
		return trace.Wrap(err)
	}
	ctx.SetValue(ContextKey(p.name), val)
	return nil
}

// MarshalCode marshals action into code representation
func (p *DefineAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Define,
		Args: []interface{}{p.name, p.value},
	}
	return call.MarshalCode(ctx)
}
