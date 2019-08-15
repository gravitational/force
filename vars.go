package force

import (
	"github.com/gravitational/trace"
)

// NewVarRef returns new variable references
type NewVarRef struct {
}

// NewInstance returns a new instance
func (n *NewVarRef) NewInstance(group Group) (Group, interface{}) {
	return group, VarRef(group)
}

// VarRef returns a new function that references a variable
// based on the defined type
func VarRef(group Group) func(name String) (interface{}, error) {
	return func(name String) (interface{}, error) {
		v, err := group.GetDefinition(string(name))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		switch v.(type) {
		case StringVar:
			return StringVarRef(string(name)), nil
		case String:
			return StringVarRef(string(name)), nil
		case IntVar:
			return IntVarRef(string(name)), nil
		case Int:
			return IntVarRef(string(name)), nil
		case BoolVar:
			return BoolVarRef(string(name)), nil
		case Bool:
			return BoolVarRef(string(name)), nil
		}
		return nil, trace.BadParameter("unsupported reference type %v", v)
	}
}

// StringVarRef returns new string variable reference
func StringVarRef(name string) StringVar {
	return StringVarFunc(func(ctx ExecutionContext) (string, error) {
		i := ctx.Value(ContextKey(name))
		if i == nil {
			return "", trace.BadParameter("variable %v is not set", name)
		}
		switch val := i.(type) {
		case string:
			return val, nil
		case StringVar:
			return val.Eval(ctx)
		}
		return "", trace.BadParameter("failed to convert variable %q to string from %T.", name, name)
	})
}

// IntVarRef returns new int variable reference
func IntVarRef(name string) IntVar {
	return IntVarFunc(func(ctx ExecutionContext) (int, error) {
		i := ctx.Value(ContextKey(name))
		if i == nil {
			return -1, trace.BadParameter("variable %v is not set", name)
		}
		switch val := i.(type) {
		case int:
			return val, nil
		case IntVar:
			return val.Eval(ctx)
		}
		return -1, trace.BadParameter("failed to convert variable %q to int from %T.", name, name)
	})
}

// BoolVarRef returns new bool variable reference
func BoolVarRef(name string) BoolVar {
	return BoolVarFunc(func(ctx ExecutionContext) (bool, error) {
		i := ctx.Value(ContextKey(name))
		if i == nil {
			return false, trace.BadParameter("variable %v is not set", name)
		}
		switch val := i.(type) {
		case bool:
			return val, nil
		case BoolVar:
			return val.Eval(ctx)
		}
		return false, trace.BadParameter("failed to convert variable %q to int from %T.", name, name)
	})
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
