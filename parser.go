package force

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/gravitational/trace"
)

// Parse returns a new instance of runner
// using G file input
func Parse(input string, runner *Runner) error {
	expr, err := parser.ParseExpr(input)
	if err != nil {
		return trace.Wrap(err)
	}
	g := &gParser{
		runner: runner,
		functions: map[string]interface{}{
			"Process":             runner.Process,
			functionName(Files):   Files,
			functionName(Command): Command,
		},
	}
	_, err = g.parseNode(expr)
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// gParser is a parser that parses G files
type gParser struct {
	runner    *Runner
	functions map[string]interface{}
}

func (g *gParser) parseNode(node ast.Node) (interface{}, error) {
	switch n := node.(type) {
	case *ast.BasicLit:
		literal, err := literalToValue(n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return literal, nil
	case *ast.CallExpr:
		// We expect function that will return predicate
		name, err := getIdentifier(n.Fun)
		if err != nil {
			return nil, err
		}
		fn, err := g.getFunction(name)
		if err != nil {
			return nil, err
		}
		arguments, err := g.evaluateArguments(n.Args)
		if err != nil {
			return nil, err
		}
		return callFunction(fn, arguments)
	case *ast.ParenExpr:
		return g.parseNode(n.X)
	}
	return nil, trace.BadParameter("unsupported %T", node)
}

func (g *gParser) evaluateArguments(nodes []ast.Expr) ([]interface{}, error) {
	out := make([]interface{}, len(nodes))
	for i, n := range nodes {
		val, err := g.evaluateExpr(n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = val
	}
	return out, nil
}

func (g *gParser) evaluateStructFields(nodes []ast.Expr) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(nodes))
	for _, n := range nodes {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return nil, trace.BadParameter("expected key value expression, got %v", n)
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, trace.BadParameter("expected value identifier, got %v", n)
		}
		val, err := g.evaluateExpr(kv.Value)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if _, exists := out[key.Name]; exists {
			return nil, trace.BadParameter("duplicate struct field %q", key.Name)
		}
		out[key.Name] = val
	}
	return out, nil
}

func (g *gParser) evaluateExpr(n ast.Expr) (interface{}, error) {
	switch l := n.(type) {
	case *ast.CompositeLit:
		ident, ok := l.Type.(*ast.Ident)
		if !ok {
			return nil, trace.BadParameter("unsupported composite literal: %v", l.Type)
		}
		structProto, err := g.getStruct(ident.Name)
		fields, err := g.evaluateStructFields(l.Elts)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		st, err := createStruct(structProto, fields)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return st, nil
	case *ast.BasicLit:
		val, err := literalToValue(l)
		if err != nil {
			return nil, err
		}
		return val, nil
	case *ast.Ident:
		val, err := getIdentifier(l)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return val, nil
	case *ast.CallExpr:
		name, err := getIdentifier(l.Fun)
		if err != nil {
			return nil, err
		}
		fn, err := g.getFunction(name)
		if err != nil {
			return nil, err
		}
		arguments, err := g.evaluateArguments(l.Args)
		if err != nil {
			return nil, err
		}
		return callFunction(fn, arguments)
	default:
		return nil, trace.BadParameter("%T is not supported", n)
	}
}

func (g *gParser) getStruct(name string) (interface{}, error) {
	switch name {
	case "Spec":
		return Spec{}, nil
	default:
		return nil, trace.BadParameter("unsupported struct: %v", name)
	}
}

func (g *gParser) getFunction(name string) (interface{}, error) {
	fn, exists := g.functions[name]
	if !exists {
		return nil, trace.BadParameter("unsupported function: %v", name, g.functions)
	}
	return fn, nil
}

func getIdentifier(node ast.Node) (string, error) {
	sexpr, ok := node.(*ast.SelectorExpr)
	if ok {
		id, ok := sexpr.X.(*ast.Ident)
		if !ok {
			return "", trace.BadParameter("expected selector identifier, got: %T", sexpr.X)
		}
		return fmt.Sprintf("%s.%s", id.Name, sexpr.Sel.Name), nil
	}
	id, ok := node.(*ast.Ident)
	if !ok {
		return "", trace.BadParameter("expected identifier, got: %T", node)
	}
	return id.Name, nil
}

func literalToValue(a *ast.BasicLit) (interface{}, error) {
	switch a.Kind {
	case token.FLOAT:
		value, err := strconv.ParseFloat(a.Value, 64)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil
	case token.INT:
		value, err := strconv.Atoi(a.Value)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil
	case token.STRING:
		value, err := strconv.Unquote(a.Value)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return value, nil
	}
	return nil, trace.BadParameter("unsupported function argument type: '%v'", a.Kind)
}

func callFunction(f interface{}, args []interface{}) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("%s", r)
		}
	}()
	arguments := make([]reflect.Value, len(args))
	for i, a := range args {
		arguments[i] = reflect.ValueOf(a)
	}
	fn := reflect.ValueOf(f)

	ret := fn.Call(arguments)
	switch len(ret) {
	case 1:
		return ret[0].Interface(), nil
	case 2:
		v, e := ret[0].Interface(), ret[1].Interface()
		if e == nil {
			return v, nil
		}
		err, ok := e.(error)
		if !ok {
			return nil, trace.BadParameter("expected error as a second return value, got %T", e)
		}
		return v, err
	}
	return nil, trace.BadParameter("expected at least one return argument for '%v'", fn)
}

func createStruct(val interface{}, args map[string]interface{}) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("%s", r)
		}
	}()
	structType := reflect.TypeOf(val)
	st := reflect.New(structType)
	for key, val := range args {
		field := st.Elem().FieldByName(key)
		if !field.IsValid() {
			return nil, trace.BadParameter("field %q not valid", key)
		}
		if !field.CanSet() {
			return nil, trace.BadParameter("can't set value of %v", field)
		}
		// TODO: fix this check to avoid potential panic below
		/*if !field.Type().AssignableTo(reflect.TypeOf(val)) {
			return nil, trace.BadParameter("can't assign %v to %v", reflect.TypeOf(val), field.Type())
		}*/
		field.Set(reflect.ValueOf(val))
	}
	return st.Elem().Interface(), nil
}

func functionName(i interface{}) string {
	fullPath := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	return strings.TrimPrefix(filepath.Ext(fullPath), ".")
}
