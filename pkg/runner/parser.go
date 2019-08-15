package runner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/git"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/kube"
	"github.com/gravitational/force/pkg/logging"

	"github.com/gravitational/trace"
)

// NewGroup creates a new group
type NewGroup struct {
}

// NewInstance returns a new instance of a function
// that does nothing but grouping methods together
func (n *NewGroup) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(vars ...interface{}) force.Group {
		return group
	}
}

// Input is an input to parser
type Input struct {
	// Scripts is a list of scripts to parse
	Scripts []string
	// Context is a global context
	Context context.Context
	// Debug turns on global debug mode
	Debug bool
}

// CheckAndSetDefaults checks and sets default values
func (i *Input) CheckAndSetDefaults() error {
	if i.Context == nil {
		return trace.BadParameter("missing parameter Context")
	}
	if len(i.Scripts) == 0 {
		return trace.BadParameter("provide at least one script to parse")
	}
	return nil
}

// Parse returns a new instance of runner
func Parse(i Input) (*Runner, error) {
	if err := i.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	ctx, cancel := context.WithCancel(i.Context)
	runner := &Runner{
		LexScope:      force.WithLexicalScope(nil),
		debugOverride: i.Debug,
		cancel:        cancel,
		ctx:           ctx,
		eventsC:       make(chan force.Event, 1024),
		plugins:       make(map[interface{}]interface{}),
	}
	g := &gParser{
		runner: runner,
		functions: map[string]force.Function{
			// Standard library functions
			"Process": &NewProcess{runner: runner},
			"Setup":   &NewGroup{},

			// Action runners
			"Sequence": &force.NewSequence{},
			"Continue": &force.NewContinue{},
			"Parallel": &force.NewParallel{},
			"Defer":    &force.NopScope{Func: force.Defer},

			// Builtin event generator channels
			"Oneshot":   &force.NopScope{Func: force.Oneshot},
			"Duplicate": &force.NopScope{Func: force.Duplicate},
			"Files":     &force.NopScope{Func: force.Files},

			// Variable-related functions
			// Define defines a variable in a lexical scope
			"Define": &force.NewDefine{},
			// Var references a variable in a lexcial scope
			"Var": &force.NewVarRef{},

			// Environment functions
			"ExpectEnv": &force.NopScope{Func: force.ExpectEnv},
			"Env":       &force.NopScope{Func: force.Env},

			// Flow control function
			"Exit": &force.NopScope{Func: force.Exit},

			// Log functions
			"Log":   &logging.NewPlugin{},
			"Infof": &force.NopScope{Func: logging.Infof},

			// Helper functions
			"Shell":     &force.NopScope{Func: force.Shell},
			"ID":        &force.NopScope{Func: force.ID},
			"Sprintf":   &force.NopScope{Func: force.Sprintf},
			"Strings":   &force.NopScope{Func: force.Strings},
			"TrimSpace": &force.NopScope{Func: force.NewTrimSpace},

			// Temp dir operators
			"TempDir":   &force.NopScope{Func: force.NewTempDir},
			"RemoveDir": &force.NopScope{Func: force.NewRemoveDir},

			// Github functions
			"Github":       &github.NewPlugin{},
			"PullRequests": &github.NewWatch{},
			"PostStatus":   &github.NewPostStatus{},
			"PostStatusOf": &github.NewPostStatusOf{},

			// Git functions
			"Git":   &git.NewPlugin{},
			"Clone": &git.NewClone{},

			// Container Builder functions
			"Builder": &builder.NewPlugin{},
			"Build":   &builder.NewBuild{},
			"Push":    &builder.NewPush{},
			"Prune":   &builder.NewPrune{},

			// Kubernetes functions
			"Kube": &kube.NewPlugin{},
			"Run":  &kube.NewRun{},
		},
		getStruct: func(name string) (interface{}, error) {
			switch name {
			// Standard library structs
			case "Test":
				return force.Test{}, nil
			case "Script":
				return force.Script{}, nil
			case "Spec":
				return force.Spec{}, nil
				// Github structs
			case "GithubConfig":
				return github.Config{}, nil
			case "Source":
				return github.Source{}, nil
				// Git structs
			case "GitConfig":
				return git.Config{}, nil
			case "Repo":
				return git.Repo{}, nil
				// Container builder structs
			case "BuilderConfig":
				return builder.Config{}, nil
			case "Image":
				return builder.Image{}, nil
			case "Secret":
				return builder.Secret{}, nil
			case "Arg":
				return builder.Arg{}, nil
				// Log structs
			case "LogConfig":
				return logging.Config{}, nil
			case "Output":
				return logging.Output{}, nil
				// Kube structs
			case "KubeConfig":
				return kube.Config{}, nil
			case "Job":
				return kube.Job{}, nil
			case "Container":
				return kube.Container{}, nil
			case "SecurityContext":
				return kube.SecurityContext{}, nil
			case "EnvVar":
				return kube.EnvVar{}, nil
			case "Volume":
				return kube.Volume{}, nil
			case "VolumeMount":
				return kube.VolumeMount{}, nil
			case "EmptyDir":
				return kube.EmptyDir{}, nil
			default:
				return nil, trace.BadParameter("unsupported struct: %v", name)
			}
		},
	}
	runner.parser = g
	for _, input := range i.Scripts {
		expr, err := parser.ParseExpr(input)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		_, err = g.parseNode(runner, expr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	// if after parsing, logging plugin is not set up
	// set it up with default plugin instance
	_, ok := runner.GetPlugin(logging.LoggingPlugin)
	if !ok {
		runner.SetPlugin(logging.LoggingPlugin, &logging.Plugin{})
	}
	return runner, nil
}

// gParser is a parser that parses G files
type gParser struct {
	runner    *Runner
	functions map[string]force.Function
	structs   map[string]interface{}
	getStruct func(name string) (interface{}, error)
}

// scope is a lexical scope of the node, new function calls
// can create new lexical scopes
func (g *gParser) parseNode(scope force.Group, node ast.Node) (interface{}, error) {
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
			return nil, trace.Wrap(err)
		}
		newFn, err := g.getFunction(name)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		// this function can create a new lexical scope
		newScope, fn := newFn.NewInstance(scope)
		// arguments should be evaluated within a new lexical scope
		arguments, err := g.evaluateArguments(newScope, n.Args)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out, err := callFunction(fn, arguments)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return out, nil
	case *ast.ParenExpr:
		return g.parseNode(scope, n.X)
	}
	return nil, trace.BadParameter("unsupported expression type %T", node)
}

func (g *gParser) evaluateArguments(scope force.Group, nodes []ast.Expr) ([]interface{}, error) {
	out := make([]interface{}, len(nodes))
	for i, n := range nodes {
		val, err := g.evaluateExpr(scope, n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = val
	}
	return out, nil
}

func (g *gParser) evaluateStructFields(scope force.Group, nodes []ast.Expr) (map[string]interface{}, error) {
	out := make(map[string]interface{}, len(nodes))
	for _, n := range nodes {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return nil, trace.BadParameter("expected key value expression, got %v", n)
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			return nil, trace.BadParameter("expected value identifier, got %#v", n)
		}
		val, err := g.evaluateExpr(scope, kv.Value)
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

func (g *gParser) evaluateExpr(scope force.Group, n ast.Expr) (interface{}, error) {
	switch l := n.(type) {
	case *ast.CompositeLit:
		switch literal := l.Type.(type) {
		case *ast.Ident:
			structProto, err := g.getStruct(literal.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			fields, err := g.evaluateStructFields(scope, l.Elts)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			st, err := createStruct(structProto, fields)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			return st, nil
		case *ast.ArrayType:
			arrayType, ok := literal.Elt.(*ast.Ident)
			if !ok {
				return nil, trace.BadParameter("unsupported composite literal: %v %T", literal.Elt, literal.Elt)
			}
			structProto, err := g.getStruct(arrayType.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			slice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(structProto)), len(l.Elts), len(l.Elts))
			for i, el := range l.Elts {
				member, ok := el.(*ast.CompositeLit)
				if !ok {
					return nil, trace.BadParameter("unsupported composite literal type: %T", l.Type)
				}
				fields, err := g.evaluateStructFields(scope, member.Elts)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				st, err := createStruct(structProto, fields)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				v := slice.Index(i)
				v.Set(reflect.ValueOf(st))
			}
			return slice.Interface(), nil
		default:
			return nil, trace.BadParameter("unsupported composite literal: %v %T", l.Type, l.Type)
		}
	case *ast.BasicLit:
		val, err := literalToValue(l)
		if err != nil {
			return nil, err
		}
		return val, nil
	case *ast.Ident:
		if l.Name == "true" {
			return force.Bool(true), nil
		} else if l.Name == "false" {
			return force.Bool(false), nil
		}
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
		newFn, err := g.getFunction(name)
		if err != nil {
			return nil, err
		}
		// new function can create a new lexical scope
		// returned with NewInstance call
		newScope, fn := newFn.NewInstance(scope)
		// evaluate arguments within a new lexical scope
		arguments, err := g.evaluateArguments(newScope, l.Args)
		if err != nil {
			return nil, err
		}
		return callFunction(fn, arguments)
	case *ast.UnaryExpr:
		if l.Op != token.AND {
			return nil, trace.BadParameter("operator %v is not supported", l.Op)
		}
		expr, err := g.evaluateExpr(scope, l.X)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if reflect.TypeOf(expr).Kind() != reflect.Struct {
			return nil, trace.BadParameter("don't know how to take address of %v", reflect.TypeOf(expr).Kind())
		}
		ptr := reflect.New(reflect.TypeOf(expr))
		ptr.Elem().Set(reflect.ValueOf(expr))
		return ptr.Interface(), nil
	default:
		return nil, trace.BadParameter("%T is not supported", n)
	}
}

func (g *gParser) getFunction(name string) (force.Function, error) {
	fn, exists := g.functions[name]
	if !exists {
		return nil, trace.BadParameter("function %v is not defined", name)
	}
	return fn, nil
}

func getIdentifier(node ast.Node) (string, error) {
	sexpr, ok := node.(*ast.SelectorExpr)
	if ok {
		id, ok := sexpr.X.(*ast.Ident)
		if !ok {
			return "", trace.BadParameter("expected selector identifier, got: %T in %#v", sexpr.X, sexpr.X)
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
		return force.Int(value), nil
	case token.STRING:
		value, err := strconv.Unquote(a.Value)
		if err != nil {
			return nil, trace.BadParameter("failed to parse argument: %s, error: %s", a.Value, err)
		}
		return force.String(value), nil
	}
	return nil, trace.BadParameter("unsupported function argument type: '%v'", a.Kind)
}

func callFunction(f interface{}, args []interface{}) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("failed calling function %v %v", functionName(f), r)
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
			err = trace.BadParameter("struct %v: %v", reflect.TypeOf(val).Name(), r)
		}
	}()
	structType := reflect.TypeOf(val)
	st := reflect.New(structType)
	for key, val := range args {
		field := st.Elem().FieldByName(key)
		if !field.IsValid() {
			return nil, trace.BadParameter("field %q is not found in %v", key, structType.Name())
		}
		if !field.CanSet() {
			return nil, trace.BadParameter("can't set value of %v", field)
		}
		field.Set(reflect.ValueOf(val))
	}
	return st.Elem().Interface(), nil
}

func functionName(i interface{}) string {
	fullPath := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	return strings.TrimPrefix(filepath.Ext(fullPath), ".")
}
