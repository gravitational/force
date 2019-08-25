package force

import (
	"fmt"
	"sync"

	"github.com/gravitational/trace"
)

// NopScope wraps func to create new instances
// not bound to new lexical scope and not referencing any scopes
type NopScope struct {
	Func interface{}
}

// NewInstance retunrs the same func not bound to any scopes
func (n *NopScope) NewInstance(group Group) (Group, interface{}) {
	return group, n.Func
}

// Function creates new functions
type Function interface {
	// NewInstance creates a new instance of the function
	// optionally creating a new lexical scope
	NewInstance(group Group) (Group, interface{})
}

// WithLexicalScope wraps a group to create a new lexical scope
func WithLexicalScope(group Group) *LexScope {
	return &LexScope{
		RWMutex: &sync.RWMutex{},
		Group:   group,
		defs:    make(map[string]interface{}),
	}
}

// LexScope wraps a group to create a new lexical scope
type LexScope struct {
	*sync.RWMutex
	Group
	defs map[string]interface{}
}

// AddDefinition defines variable to track its type
// the variable is set on the execution context
func (l *LexScope) AddDefinition(name string, v interface{}) error {
	l.Lock()
	defer l.Unlock()
	if name == "" {
		return trace.BadParameter("provide variable name")
	}
	if v == nil {
		return trace.BadParameter("specify vairiable %v value", name)
	}
	if _, ok := l.defs[name]; ok {
		return trace.BadParameter("variable %v is already defined", name)
	}
	err := checkDefinedType(v)
	if err != nil {
		return trace.Wrap(err)
	}
	l.defs[name] = v
	return nil
}

// GetDefinition gets a variable defined with DefineVarType
// not the actual variable value is returned, but a prototype
// value specifying the type
func (l *LexScope) GetDefinition(name string) (interface{}, error) {
	if name == "" {
		return nil, trace.BadParameter("provide variable name")
	}
	l.RLock()
	v, ok := l.defs[name]
	l.RUnlock()
	if ok {
		return v, nil
	}
	if l.Group == nil {
		if trace.IsDebug() {
			return nil, trace.NotFound("variable %v is not defined, defined are %v", name, l.Variables())
		}
		return nil, trace.NotFound("variable %v is not defined", name)
	}
	return l.Group.GetDefinition(name)
}

// Variables returns all variables defined in this scope
// (and parent scopes)
func (l *LexScope) Variables() []string {
	var out []string
	l.RLock()
	for key := range l.defs {
		out = append(out, key)
	}
	l.RUnlock()
	if l.Group == nil {
		return out
	}
	parent := l.Group.Variables()
	for _, p := range parent {
		out = append(out, fmt.Sprintf("p(%v)", p))
	}
	return out
}

func checkDefinedType(v interface{}) error {
	switch v.(type) {
	case StringVar:
		return nil
	case String:
		return nil
	case IntVar:
		return nil
	case Int:
		return nil
	case BoolVar:
		return nil
	case Bool:
		return nil
	case Action:
		return nil
	}
	return trace.BadParameter("%T is not a supported variable type, supported are int, bool and string", v)
}
