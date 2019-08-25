package runner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strconv"
	"time"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/git"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/kube"
	"github.com/gravitational/force/pkg/logging"

	"github.com/gravitational/trace"
)

// Input is an input to parser
type Input struct {
	// ID specifies run ID
	ID string
	// Setup is an optional setup script to parse
	// it sets up a group of processes
	Setup string
	// Script is a script to parse
	Script string
	// IncludeScripts is a set of scripts to include
	IncludeScripts []string
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
	if i.ID == "" {
		i.ID = ShortID()
	}
	if len(i.Script) == 0 {
		return trace.BadParameter("missing parameter Script")
	}
	return nil
}

// Parse parses golang-like expressions, for example:
//
// Infof("hello")
//
// And calls registered function "Infof" with argument "hello",
// however, Infof does not immediatelly log the "hello" message,
// instead it returns LogAction{Message: "hello"}, essentially
//
// translating golang syntax into a tree of actions to interpret,
// like a high level virtual machine, for example
//
// func(){
//    a := "hello"
//    Infof(a)
// }()
//
// becomes:
//
// LambdaFunctionCall{
//    LambdaFunction{
//         Statements: {
//              DefineAction{Name: "a", Value: "hello"},
//              InfofAction{Format: VariableReference{Name: "a"}}
//         }
//    }
//    // called with no args
//    Args: {}
//  }
//
//
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
	var builtinFunctions = map[string]force.Function{
		// Standard library functions
		"Process": &NewProcess{runner: runner},
		"Setup":   &NewSetupProcess{runner: runner},

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
		"Command":   &force.NopScope{Func: force.Command},
		"ID":        &force.NopScope{Func: force.ID},
		"Sprintf":   &force.NopScope{Func: force.Sprintf},
		"Strings":   &force.NopScope{Func: force.Strings},
		"TrimSpace": &force.NopScope{Func: force.TrimSpace},
		"Marshal":   &force.NopScope{Func: force.Marshal},
		"Unquote":   &force.NopScope{Func: force.Unquote},

		// Temp dir operators
		"TempDir":    &force.NopScope{Func: force.TempDir},
		"CurrentDir": &force.NopScope{Func: force.CurrentDir},
		"RemoveDir":  &force.NopScope{Func: force.RemoveDir},

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
	}

	g := &gParser{
		runner: runner,
		scope: force.WithRuntimeScope(force.NewContext(force.ContextConfig{
			Context: i.Context,
			Process: nil,
			ID:      i.ID,
			Event:   &force.OneshotEvent{Time: time.Now().UTC()},
		})),
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
				return github.GithubConfig{}, nil
			case "Source":
				return github.Source{}, nil
				// Git structs
			case "GitConfig":
				return git.GitConfig{}, nil
			case "Repo":
				return git.Repo{}, nil
				// Container builder structs
			case "BuilderConfig":
				return builder.BuilderConfig{}, nil
			case "Image":
				return builder.Image{}, nil
			case "Secret":
				return builder.Secret{}, nil
			case "Arg":
				return builder.Arg{}, nil
				// Log structs
			case "LogConfig":
				return logging.LogConfig{}, nil
			case "Output":
				return logging.Output{}, nil
				// Kube structs
			case "KubeConfig":
				return kube.KubeConfig{}, nil
			case "Job":
				return kube.Job{}, nil
			case "Container":
				return kube.Container{}, nil
			case "PodSecurityContext":
				return kube.PodSecurityContext{}, nil
			case "SecurityContext":
				return kube.SecurityContext{}, nil
			case "EnvVar":
				return kube.EnvVar{}, nil
			case "Volume":
				return kube.Volume{}, nil
			case "VolumeMount":
				return kube.VolumeMount{}, nil
			case "EmptyDirSource":
				return kube.EmptyDirSource{}, nil
			case "SecretSource":
				return kube.SecretSource{}, nil
			case "ConfigMapSource":
				return kube.ConfigMapSource{}, nil
			default:
				return nil, trace.BadParameter("unsupported struct: %v", name)
			}
		},
	}
	runner.parser = g
	for name, fn := range builtinFunctions {
		g.scope.SetValue(force.ContextKey(name), fn)
	}
	// Setup the runner
	if i.Setup != "" {
		expr, err := parser.ParseExpr(i.Setup)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		procI, err := g.parseExpr(runner, expr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		proc, ok := procI.(force.Process)
		if !ok {
			return nil, trace.BadParameter("expected Setup")
		}
		// create a local setup context and run the setup process
		setupContext := force.NewContext(force.ContextConfig{
			Context: i.Context,
			Process: proc,
			ID:      i.ID,
			Event:   &force.OneshotEvent{Time: time.Now().UTC()},
		})
		if err := proc.Action().Run(setupContext); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	for _, script := range i.IncludeScripts {
		expr, err := parser.ParseExpr(script)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		actionI, err := g.parseExpr(runner, expr)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		action, ok := actionI.(force.ScopeAction)
		if !ok {
			return nil, trace.BadParameter("expected scope action, got %T", actionI)
		}
		err = action.RunWithScope(g.scope)
		if err != nil {
			return nil, trace.Wrap(err)
		}
	}

	expr, err := parser.ParseExpr(i.Script)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	procI, err := g.parseExpr(runner, expr)
	if err != nil {
		return nil, trace.Wrap(err)
	}

	proc, ok := procI.(force.Process)
	if !ok {
		return nil, trace.BadParameter("expected Process or Setup, got something else")
	}

	// if after parsing, logging plugin is not set up
	// set it up with default plugin instance
	_, ok = runner.GetPlugin(logging.LoggingPlugin)
	if !ok {
		runner.SetPlugin(logging.LoggingPlugin, &logging.Plugin{})
	}

	runner.AddProcess(proc)
	runner.Logger().Debugf("Add event source %v.", proc.Channel())
	runner.AddChannel(proc.Channel())

	return runner, nil
}

// gParser is a parser that parses G files
type gParser struct {
	runner    *Runner
	scope     *force.RuntimeScope
	structs   map[string]interface{}
	getStruct func(name string) (interface{}, error)
}

func (g *gParser) parseArguments(scope force.Group, nodes []ast.Node) ([]interface{}, error) {
	out := make([]interface{}, len(nodes))
	for i, n := range nodes {
		val, err := g.parseExpr(scope, n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		out[i] = val
	}
	return out, nil
}

func (g *gParser) parseStatements(scope force.Group, nodes []ast.Node) ([]force.Action, error) {
	out := make([]force.Action, len(nodes))
	for i, n := range nodes {
		val, err := g.parseExpr(scope, n)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		statement, ok := val.(force.Action)
		if !ok {
			return nil, trace.BadParameter("expected statement, got %v instead", val)
		}
		out[i] = statement
	}
	return out, nil
}

func (g *gParser) parseStructFields(scope force.Group, nodes []ast.Expr) (map[string]interface{}, error) {
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
		val, err := g.parseExpr(scope, kv.Value)
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

func (g *gParser) parseExpr(scope force.Group, n ast.Node) (interface{}, error) {
	switch l := n.(type) {
	case *ast.ParenExpr:
		return g.parseExpr(scope, l.X)
	case *ast.CompositeLit:
		switch literal := l.Type.(type) {
		case *ast.Ident:
			structProto, err := g.getStruct(literal.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			fields, err := g.parseStructFields(scope, l.Elts)
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
				fields, err := g.parseStructFields(scope, member.Elts)
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
		return force.Var(scope)(force.String(val))
	case *ast.CallExpr:
		var newFn force.Function
		// could be inline function call
		var err error
		var newScope force.Group
		var fn interface{}
		switch call := l.Fun.(type) {
		case *ast.Ident:
			newFn, err = g.getFunction(call.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			// new function can create a new lexical scope
			// returned with NewInstance call
			newScope, fn = newFn.NewInstance(scope)
		case *ast.FuncLit:
			expr, err := g.parseExpr(scope, l.Fun)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			var ok bool
			newFn, ok = expr.(force.Function)
			if !ok {
				return nil, trace.BadParameter("expected lambda function, got %v instead", expr)
			}
			newScope, fn = newFn.NewInstance(scope)
		}
		// evaluate arguments within a new lexical scope
		nodes := make([]ast.Node, len(l.Args))
		for i := range l.Args {
			nodes[i] = l.Args[i]
		}
		arguments, err := g.parseArguments(newScope, nodes)
		if err != nil {
			return nil, err
		}
		lambda, ok := newFn.(*force.LambdaFunction)
		if !ok {
			return callFunction(fn, arguments)
		}
		if len(arguments) != len(lambda.Params) {
			return nil, trace.BadParameter("expected %v params, found %v", len(lambda.Params), len(arguments))
		}
		// in case of lambda function, passed arguments
		// are converted into defined statements
		callArgs := make([]force.Action, len(arguments))
		for i, param := range lambda.Params {
			def, err := force.Define(lambda.Scope)(force.String(param.Name), arguments[i])
			if err != nil {
				return nil, trace.Wrap(err)
			}
			callArgs[i] = def
		}
		return &force.LambdaFunctionCall{
			LambdaFunction: *lambda,
			Arguments:      callArgs,
		}, nil
	case *ast.AssignStmt:
		if len(l.Lhs) != 1 {
			return nil, trace.BadParameter("multiple assignment expressions are not supported")
		}
		id, ok := l.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, trace.BadParameter("expected identifier, got %T", l.Lhs[0])
		}
		newFn, err := g.getFunction("Define")
		if err != nil {
			return nil, trace.Wrap(err)
		}
		// new function can create a new lexical scope
		// returned with NewInstance call
		newScope, fn := newFn.NewInstance(scope)
		// evaluate arguments within a new lexical scope
		nodes := make([]ast.Node, len(l.Rhs))
		for i := range l.Rhs {
			nodes[i] = l.Rhs[i]
		}
		arguments, err := g.parseArguments(newScope, nodes)
		if err != nil {
			return nil, err
		}
		arguments = append([]interface{}{force.String(id.Name)}, arguments...)
		return callFunction(fn, arguments)
	case *ast.UnaryExpr:
		if l.Op != token.AND {
			return nil, trace.BadParameter("operator %v is not supported", l.Op)
		}
		expr, err := g.parseExpr(scope, l.X)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if reflect.TypeOf(expr).Kind() != reflect.Struct {
			return nil, trace.BadParameter("don't know how to take address of %v", reflect.TypeOf(expr).Kind())
		}
		ptr := reflect.New(reflect.TypeOf(expr))
		ptr.Elem().Set(reflect.ValueOf(expr))
		return ptr.Interface(), nil
	case *ast.FuncLit:
		if l.Type.Results != nil && len(l.Type.Results.List) != 0 {
			return nil, trace.BadParameter("functions with return values are not supported")
		}
		lambda := &force.LambdaFunction{
			Scope: force.WithLexicalScope(scope),
		}
		if l.Type.Params != nil && len(l.Type.Params.List) != 0 {
			for i, p := range l.Type.Params.List {
				if len(p.Names) != 1 {
					return nil, trace.BadParameter("lambda function parameter %v name is not supported", i)
				}
				arg, err := g.evalFunctionArg(p.Type)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				param := force.LambdaParam{Name: p.Names[0].Name, Prototype: arg}
				lambda.Params = append(lambda.Params, param)
				if err := scope.AddDefinition(param.Name, arg); err != nil {
					return nil, trace.Wrap(err)
				}
			}
		}
		// evaluate arguments within a new lexical scope
		nodes := make([]ast.Node, len(l.Body.List))
		for i := range l.Body.List {
			nodes[i] = l.Body.List[i]
		}
		var err error
		lambda.Statements, err = g.parseStatements(lambda.Scope, nodes)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return lambda, nil
	case *ast.ExprStmt:
		return g.parseExpr(scope, l.X)
	default:
		return nil, trace.BadParameter("%T is not supported", n)
	}
}

func (g *gParser) evalFunctionArg(n ast.Node) (interface{}, error) {
	switch l := n.(type) {
	case *ast.Ident:
		return literalZeroValue(l.Name)
	default:
		return nil, trace.BadParameter("%T is not supported", n)
	}
}

func (g *gParser) getFunction(name string) (force.Function, error) {
	fnI := g.scope.Value(force.ContextKey(name))
	if fnI == nil {
		return nil, trace.BadParameter("function %v is not defined", name)
	}
	fn, ok := fnI.(force.Function)
	if !ok {
		return nil, trace.BadParameter("function %v is not a variable", name)
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

func literalZeroValue(kind string) (interface{}, error) {
	switch kind {
	case "string":
		return force.String(""), nil
	case "int":
		return force.Int(0), nil
	case "bool":
		return force.Bool(false), nil
	}
	return nil, trace.BadParameter("unsupported function argument type: '%v'", kind)
}

func callFunction(f interface{}, args []interface{}) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("failed calling function %v with args %#v %v", force.FunctionName(f), args, r)
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
