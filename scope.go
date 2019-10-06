package force

import (
	"fmt"
	"reflect"
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

// WithParent wraps a group to create a new lexical scope
func WithParent(group Group, parent interface{}) *LexScope {
	return &LexScope{
		RWMutex: &sync.RWMutex{},
		Group:   group,
		defs:    make(map[string]interface{}),
		parent:  parent,
	}
}

// LexScope wraps a group to create a new lexical scope
type LexScope struct {
	*sync.RWMutex
	Group
	defs   map[string]interface{}
	parent interface{}
}

// ImportStructsIntoAST converts structs to AST compatible
// types and registers definitions in the
func ImportStructsIntoAST(l *LexScope, structs ...reflect.Type) error {
	for i := range structs {
		t := structs[i]
		if t.Kind() != reflect.Struct {
			return trace.BadParameter("expected %v to be struct, got %v", t, t.Kind())
		}
		out, err := ConvertTypeToAST(t)
		if err != nil {
			return trace.Wrap(err)
		}
		structName := StructName(out)
		if structName == "" {
			return trace.BadParameter("failed to get struct name from %T", out)
		}
		_, err = l.GetDefinition(structName)
		if trace.IsNotFound(err) {
			if err := l.AddDefinition(structName, out); err != nil {
				return trace.Wrap(err)
			}
		} else if err != nil {
			return trace.Wrap(err)
		}
		for j := 0; j < out.NumField(); j++ {
			field := out.Field(j)
			if field.Tag.Get(codeTag) == codeSkip {
				continue
			}
			switch field.Type.Kind() {
			case reflect.Ptr:
				if field.Type.Elem().Kind() == reflect.Struct {
					if err := ImportStructsIntoAST(l, field.Type.Elem()); err != nil {
						return trace.Wrap(err)
					}
				}
			case reflect.Struct:
				if err := ImportStructsIntoAST(l, field.Type); err != nil {
					return trace.Wrap(err)
				}
			case reflect.Slice:
				if field.Type.Elem().Kind() == reflect.Struct {
					if err := ImportStructsIntoAST(l, field.Type.Elem()); err != nil {
						return trace.Wrap(err)
					}
				}
			}
		}
	}
	return nil
}

// SetParent returns a parent definition of the lexical scope
func (l *LexScope) SetParent(p interface{}) {
	l.Lock()
	defer l.Unlock()
	l.parent = p
}

// GetParent returns a parent definition of the lexical scope
func (l *LexScope) GetParent() (interface{}, error) {
	l.RLock()
	defer l.RUnlock()
	if l.parent == nil {
		return nil, trace.NotFound("scope has no defined parent")
	}
	return l.parent, nil
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
			return nil, trace.NotFound("%v is not defined, defined are %v", name, l.Variables())
		}
		return nil, trace.NotFound("%v is not defined", name)
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
