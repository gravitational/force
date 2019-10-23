package runner

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gravitational/force"

	"github.com/gravitational/force/pkg/aws"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/git"
	"github.com/gravitational/force/pkg/github"
	"github.com/gravitational/force/pkg/kube"
	"github.com/gravitational/force/pkg/log"
	"github.com/gravitational/force/pkg/slack"
	"github.com/gravitational/force/pkg/ssh"

	"github.com/gravitational/trace"
)

// Script is a force code script
type Script struct {
	// Filename is a file name of the script
	Filename string
	// Content is a script content
	Content string
}

// Input is an input to parser
type Input struct {
	// ID specifies run ID
	ID string
	// Setup is an optional setup script to parse
	// it sets up a group of processes
	Setup Script
	// Script is a script to parse
	Script Script
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
	if len(i.Script.Content) == 0 {
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
		runners:       make(map[string]*Runner),
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
		"Parallel": &force.NewParallel{},
		"Defer":    &force.NopScope{Func: force.Defer},
		"If":       &force.NewIf{},

		// Builtin event generator channels
		"Oneshot":   &force.NopScope{Func: force.Oneshot},
		"Ticker":    &force.NopScope{Func: force.Ticker},
		"Duplicate": &force.NopScope{Func: force.Duplicate},
		"Files":     &force.NopScope{Func: force.Files},

		// Variable-related functions
		// Define defines a variable in a lexical scope
		"Define": &force.NewDefine{},
		// Var references a variable in a lexcial scope
		"Var": &force.NewVarRef{},

		// Flow control function
		"Exit": &force.NopScope{Func: force.Exit},

		// Log functions
		"Infof": &force.NopScope{Func: log.Infof},

		// Helper functions
		"Shell":    &force.NopScope{Func: force.Shell},
		"Command":  &force.NopScope{Func: force.Command},
		"ID":       &force.NopScope{Func: force.ID},
		"Strings":  &force.NopScope{Func: force.Strings},
		"Marshal":  &force.NopScope{Func: force.Marshal},
		"Unquote":  &force.NopScope{Func: force.Unquote},
		"Contains": &force.NopScope{Func: force.Contains},
	}

	var builtinStructs = []interface{}{force.Spec{}, force.Test{}, force.Script{}}

	globalContext := force.NewContext(force.ContextConfig{
		Parent:  &force.WrapContext{Context: runner.ctx},
		Process: nil,
		ID:      i.ID,
		Event:   &force.OneshotEvent{Time: time.Now().UTC()},
	})
	g := &gParser{
		runner:  runner,
		scope:   force.WithRuntimeScope(globalContext),
		plugins: map[string]force.Group{},
	}
	plugins := map[string]func() (force.Group, error){
		string(log.Key):     log.Scope,
		string(git.Key):     git.Scope,
		string(github.Key):  github.Scope,
		string(slack.Key):   slack.Scope,
		string(builder.Key): builder.Scope,
		string(kube.Key):    kube.Scope,
		string(ssh.Key):     ssh.Scope,
		string(aws.Key):     aws.Scope,
	}
	for key, plugin := range plugins {
		scope, err := plugin()
		if err != nil {
			return nil, trace.Wrap(err)
		}
		g.plugins[key] = scope
	}

	runner.parser = g
	for name, fn := range builtinFunctions {
		g.scope.SetValue(force.ContextKey(name), fn)
	}

	// some parsing builtins
	g.scope.SetValue(force.ContextKey(force.FunctionName(g.Include)), &force.NopScope{Func: g.Include})
	g.scope.SetValue(force.ContextKey(force.FunctionName(g.Load)), &force.NopScope{Func: g.Load})
	g.scope.SetValue(force.ContextKey(force.FunctionName(g.Reload)), &force.NopScope{Func: g.Reload})

	// imported standard functions
	importedFunctions := []interface{}{
		fmt.Sprintf,
		strings.TrimSpace,
		os.Getenv,
		os.Getwd,
		force.ExpectEnv,
		ioutil.TempDir,
		os.RemoveAll,
	}
	for _, fn := range importedFunctions {
		outFn, err := force.ConvertFunctionToAST(fn)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		g.scope.SetValue(force.ContextKey(force.FunctionName(fn)), outFn)
	}

	for _, st := range builtinStructs {
		g.runner.AddDefinition(force.StructName(reflect.TypeOf(st)), reflect.TypeOf(st))
	}

	// Setup the runner
	if i.Setup.Content != "" {
		f := token.NewFileSet()
		expr, err := parser.ParseExprFrom(f, "", []byte(i.Setup.Content), 0)
		if err != nil {
			return nil, trace.Wrap(convertScanError(err, i.Setup))
		}
		procI, err := g.parseExpr(f, runner, expr)
		if err != nil {
			return nil, trace.Wrap(convertScanError(err, i.Setup))
		}
		proc, ok := procI.(force.Process)
		if !ok {
			return nil, trace.BadParameter("expected Setup")
		}
		// create a local setup context and run the setup process
		setupContext := force.NewContext(force.ContextConfig{
			Parent:  &force.WrapContext{Context: i.Context},
			Process: proc,
			ID:      i.ID,
			Event:   &force.OneshotEvent{Time: time.Now().UTC()},
		})
		if _, err := proc.Action().Eval(setupContext); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	f := token.NewFileSet()
	expr, err := parser.ParseExprFrom(f, "", []byte(i.Script.Content), 0)
	if err != nil {
		return nil, trace.Wrap(convertScanError(err, i.Script))
	}

	procI, err := g.parseExpr(f, runner, expr)
	if err != nil {
		return nil, trace.Wrap(convertScanError(err, i.Script))
	}

	var proc force.Process
	switch v := procI.(type) {
	case force.Process:
		proc = v
	case force.Action:
		out, err := runner.OneshotWithExit(v)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		proc = out
	default:
		return nil, trace.BadParameter("expected Process or Setup, got something else: %T", procI)
	}

	// if after parsing, logging plugin is not set up
	// set it up with default plugin instance
	_, ok := runner.GetPlugin(log.Key)
	if !ok {
		runner.SetPlugin(log.Key, &log.Plugin{})
	}

	runner.AddProcess(proc)
	runner.Logger().Debugf("Add event source %v.", proc.Channel())
	runner.AddChannel(proc.Channel())

	return runner, nil
}

// convertScanError converts scan error to code error
func convertScanError(e error, script Script) error {
	switch err := trace.Unwrap(e).(type) {
	case *force.CodeError:
		err.Snippet.Pos.Filename = script.Filename
		err.Snippet = force.CaptureSnippet(err.Snippet.Pos, script.Content)
		return e
	case scanner.ErrorList:
		var errors []error
		for _, sub := range err {
			sub.Pos.Filename = script.Filename
			snippet := force.CaptureSnippet(sub.Pos, script.Content)
			errors = append(errors,
				&force.CodeError{Err: trace.BadParameter(sub.Msg), Snippet: snippet})
		}
		return trace.NewAggregate(errors...)
	default:
		return e
	}
}

// gParser is a parser that parses G files
type gParser struct {
	runner  *Runner
	scope   *force.RuntimeScope
	plugins map[string]force.Group
}

// Reload action parses the process at a given file
// and waits until the process completes, when called twice
// it shuts down the previous
func (g *gParser) Reload(path force.Expression) (force.Action, error) {
	if err := force.ExpectString(path); err != nil {
		return nil, trace.Wrap(err)
	}
	return &LoadAction{
		reload: true,
		g:      g,
		path:   path,
	}, nil
}

// Load starts a sub process by parsing the file,
// and waits until the process completes
func (g *gParser) Load(path force.Expression) (force.Action, error) {
	if err := force.ExpectString(path); err != nil {
		return nil, trace.Wrap(err)
	}
	// include action has to be run right away,
	// so it can define the methods
	return &LoadAction{
		g:    g,
		path: path,
	}, nil
}

// Include includes and evaluates script by given paths,
// it does parsing at a parsing time, not at the exectuion time,
// that is why it evaluates with the global scope
// and returns a NopAction placeholder that does nothing at the runtime
// in this sense, Include is more like a macro preprocessing directive
func (g *gParser) Include(paths ...force.Expression) (force.Action, error) {
	for _, p := range paths {
		if err := force.ExpectString(p); err != nil {
			return nil, trace.Wrap(err)
		}
	}
	// include action has to be run right away,
	// so it can define the methods
	action := &IncludeAction{
		g:     g,
		paths: paths,
	}
	if _, err := action.Eval(g.scope); err != nil {
		return nil, trace.Wrap(err)
	}
	return &force.NopAction{
		FnName:   force.FunctionName(g.Include),
		Args:     paths,
		EvalType: "",
	}, nil
}

func (g *gParser) parseArguments(f *token.FileSet, scope force.Group, nodes []ast.Node, argumentTypes []reflect.Type) ([]interface{}, error) {
	out := make([]interface{}, len(nodes))
	for i, n := range nodes {
		var argType reflect.Type
		// assume it's a variadic arg call
		if i > len(argumentTypes)-1 {
			if len(argumentTypes) == 0 {
				return nil, wrap(f, n, trace.BadParameter("function does not accept any arguments"))
			}
			argType = argumentTypes[len(argumentTypes)-1]
		} else {
			argType = argumentTypes[i]
		}
		val, err := g.parseExpr(f, force.WithParent(scope, argType), n)
		if err != nil {
			return nil, wrap(f, n, trace.Wrap(err))
		}
		out[i] = val
	}
	return out, nil
}

func (g *gParser) parseStatements(f *token.FileSet, scope force.Group, nodes []ast.Node) ([]force.Action, error) {
	out := make([]force.Action, len(nodes))
	for i, n := range nodes {
		val, err := g.parseExpr(f, scope, n)
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

func (g *gParser) parseStructFields(f *token.FileSet, scope force.Group, nodes []ast.Expr) (map[string]interface{}, error) {
	parent, err := scope.GetParent()
	if err != nil {
		return nil, trace.Wrap(err)
	}
	parentType := parent.(reflect.Type)
	structScope := force.WithLexicalScope(scope)
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
		var structField *reflect.StructField
		var err error
		if parentType.Kind() == reflect.Struct {
			structField, err = fieldTypeByName(parentType, key.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
		} else if parentType.Kind() == reflect.Map {
			structField = &reflect.StructField{
				Name: key.Name,
				Type: parentType.Elem(),
			}
		} else {
			return nil, trace.BadParameter("unsupported type %v", parentType)
		}
		//
		// this scope is shared between struct fields, because in Force, the following is possible:
		// PullRequests defines a new event type
		// further references in the Run, so in struct definition
		// Fields are sharing the same lexical scope
		//
		// Spec: {Watch: PullReqeuests(), Run: func(){event.Commit}}
		structScope.SetParent(structField.Type)
		val, err := g.parseExpr(f, structScope, kv.Value)
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

func (g *gParser) parseExpr(f *token.FileSet, scope force.Group, n ast.Node) (interface{}, error) {
	switch l := n.(type) {
	case *ast.ParenExpr:
		return g.parseExpr(f, scope, l.X)
	case *ast.CompositeLit:
		switch literal := l.Type.(type) {
		case *ast.SelectorExpr:
			module, ok := literal.X.(*ast.Ident)
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("expected identifier, got %T", literal.X))
			}
			plugin, ok := g.plugins[module.Name]
			if !ok {
				return nil, trace.BadParameter("plugin %v is not found", module.Name)
			}
			structProto, err := plugin.GetDefinition(literal.Sel.Name)
			if err != nil {
				return nil, wrap(f, n, trace.Wrap(err))
			}
			fields, err := g.parseStructFields(f, force.WithParent(scope, structProto), l.Elts)
			if err != nil {
				return nil, wrap(f, n, trace.Wrap(err))
			}
			st, err := createStruct(structProto, fields, false)
			if err != nil {
				return nil, wrap(f, n, trace.Wrap(err))
			}
			return st, nil
		case *ast.Ident:
			var protoType reflect.Type
			var err error
			// assume this is a shortcut
			var isPointer bool
			if literal.Name == force.Underscore {
				parent, err := scope.GetParent()
				if err != nil {
					return nil, wrap(f, n, trace.NotFound("not enough information to infer type"))
				}
				parentType, ok := parent.(reflect.Type)
				if !ok {
					return nil, wrap(f, n, trace.NotFound("unsupported type got %T", parent))
				}
				if parentType.Kind() == reflect.Ptr {
					protoType = parentType.Elem()
					isPointer = true
				} else {
					protoType = parentType
				}
			} else {
				out, err := g.runner.GetDefinition(literal.Name)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
				protoType = out.(reflect.Type)
			}
			fields, err := g.parseStructFields(f, force.WithParent(scope, protoType), l.Elts)
			if err != nil {
				return nil, wrap(f, n, trace.Wrap(err))
			}
			switch protoType.Kind() {
			case reflect.Struct:
				st, err := createStruct(protoType, fields, isPointer)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
				return st, nil
			case reflect.Map:
				m := reflect.MakeMapWithSize(protoType, len(fields))
				converter, ok := reflect.Zero(protoType.Elem()).Interface().(force.Converter)
				for key := range fields {
					if ok {
						out, err := converter.Convert(fields[key])
						if err != nil {
							return nil, wrap(f, n, trace.Wrap(err))
						}
						m.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(out))
					} else {
						m.SetMapIndex(reflect.ValueOf(key), reflect.ValueOf(fields[key]))
					}
				}
				return m.Interface(), nil
			default:
				return nil, wrap(f, n,
					trace.BadParameter("can not convert type %v to map or struct", protoType.Kind()))
			}
		case *ast.ArrayType:
			var structProto interface{}
			var err error
			switch arrayType := literal.Elt.(type) {
			case *ast.Ident:
				// Underscore triggers attempt at type inference
				if arrayType.Name == force.Underscore {
					parent, err := scope.GetParent()
					if err != nil {
						return nil, wrap(f, n, trace.NotFound("underscore is supported either in struct definitions, or function calls"))
					}
					structType, ok := parent.(reflect.Type)
					if !ok {
						return nil, wrap(f, n, trace.NotFound("underscore is supported either in struct definitions or function calls, got %T", parent))
					}
					if structType.Kind() != reflect.Slice {
						return nil, wrap(f, n, trace.NotFound("expected slice, got %v", structType.Kind()))
					}
					structProto = structType.Elem()
				} else {
					structProto, err = g.runner.GetDefinition(arrayType.Name)
					if err != nil {
						return nil, wrap(f, n, trace.Wrap(err))
					}
				}
			case *ast.SelectorExpr:
				module, ok := arrayType.X.(*ast.Ident)
				if !ok {
					return nil, wrap(f, n, trace.BadParameter("expected identifier, got %T", arrayType.X))
				}
				plugin, ok := g.plugins[module.Name]
				if !ok {
					return nil, wrap(f, n, trace.BadParameter("plugin %v is not found", module.Name))
				}
				structProto, err = plugin.GetDefinition(arrayType.Sel.Name)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
			default:
				return nil, wrap(f, n, trace.BadParameter("unsupported composite literal: %v %T", literal.Elt, literal.Elt))
			}
			structType, ok := structProto.(reflect.Type)
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("expected type, got %T", structProto))
			}
			slice := reflect.MakeSlice(reflect.SliceOf(structType), len(l.Elts), len(l.Elts))
			for i, el := range l.Elts {
				member, ok := el.(*ast.CompositeLit)
				if !ok {
					return nil, wrap(f, n, trace.BadParameter("unsupported composite literal type: %T", l.Type))
				}
				fields, err := g.parseStructFields(f, force.WithParent(scope, structType), member.Elts)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
				st, err := createStruct(structProto, fields, false)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
				v := slice.Index(i)
				v.Set(reflect.ValueOf(st))
			}
			return slice.Interface(), nil
		default:
			return nil, wrap(f, n, trace.BadParameter("unsupported composite literal: %v %T", l.Type, l.Type))
		}
	case *ast.BasicLit:
		val, err := literalToValue(l)
		if err != nil {
			return nil, wrap(f, n, err)
		}
		return val, nil
	case *ast.Ident:
		if l.Name == "true" {
			return force.Bool(true), nil
		} else if l.Name == "false" {
			return force.Bool(false), nil
		}
		val, err := getIdentifier(f, l)
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
			newFn, err = g.getFunction(scope, call.Name)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			// new function can create a new lexical scope
			// returned with NewInstance call
			newScope, fn = newFn.NewInstance(scope)
		case *ast.FuncLit:
			expr, err := g.parseExpr(f, scope, l.Fun)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			var ok bool
			newFn, ok = expr.(force.Function)
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("expected lambda function, got %v instead", expr))
			}
			newScope, fn = newFn.NewInstance(scope)
		case *ast.SelectorExpr:
			module, ok := call.X.(*ast.Ident)
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("expected identifier, got %T", call.X))
			}
			plugin, ok := g.plugins[module.Name]
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("plugin %v is not found", module.Name))
			}
			fnI, err := plugin.GetDefinition(call.Sel.Name)
			if err != nil {
				return nil, wrap(f, n, trace.Wrap(err))
			}
			newFn, ok := fnI.(force.Function)
			if !ok {
				return nil, wrap(f, n, trace.BadParameter("expected function, got %T for call %v.%v", fnI, call.X, call.Sel.Name))
			}
			newScope, fn = newFn.NewInstance(scope)
		default:
			return nil, wrap(f, n, trace.BadParameter("unsupported function %T", l.Fun))
		}
		// evaluate arguments within a new lexical scope
		nodes := make([]ast.Node, len(l.Args))
		for i := range l.Args {
			nodes[i] = l.Args[i]
		}
		// argumentTypes are used to check argument types
		var argumentTypes []reflect.Type
		// lambdaExpression is not the same as lambdaType,
		// because at this point only lambdaType is known,
		// the actual evaluated function is not
		var lambdaExpression force.Expression
		var lambdaType *force.LambdaFunction
		switch fnType := newFn.(type) {
		// LambdaFunction
		case *force.LambdaFunction:
			lambdaType, lambdaExpression = fnType, fnType
		case force.Expression:
			// Make sure that expression evaluates
			var err error
			lambdaType, err = force.ExpectLambdaFunction(fnType)
			if err != nil {
				return nil, wrap(f, n, err)
			}
			lambdaExpression = fnType
		default:
			// standard function call
		}
		if lambdaType != nil {
			argumentTypes = make([]reflect.Type, len(lambdaType.Params))
			for i := range lambdaType.Params {
				argumentTypes[i] = reflect.TypeOf(lambdaType.Params[i].Prototype)
			}
		} else {
			fnType := reflect.TypeOf(fn)
			argumentTypes = make([]reflect.Type, fnType.NumIn())
			for i := 0; i < fnType.NumIn(); i++ {
				argumentTypes[i] = fnType.In(i)
			}
		}
		arguments, err := g.parseArguments(f, newScope, nodes, argumentTypes)
		if err != nil {
			return nil, wrap(f, n, err)
		}
		if lambdaType == nil {
			return callFunction(fn, arguments)
		}
		call := &force.LambdaFunctionCall{
			Expression: lambdaExpression,
			Arguments:  arguments,
		}
		if err := call.CheckCall(); err != nil {
			return nil, trace.Wrap(err)
		}
		return call, nil
	case *ast.AssignStmt:
		if len(l.Lhs) != 1 || len(l.Rhs) != 1 {
			return nil, wrap(f, n, trace.BadParameter("multiple assignment expressions are not supported"))
		}
		id, ok := l.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, wrap(f, n, trace.BadParameter("expected identifier, got %T", l.Lhs[0]))
		}
		value, err := g.parseExpr(f, scope, l.Rhs[0])
		if err != nil {
			return nil, wrap(f, n, trace.Wrap(err))
		}
		return force.Define(scope)(force.String(id.Name), value)
	case *ast.UnaryExpr:
		if l.Op != token.AND {
			return nil, wrap(f, n, trace.BadParameter("operator %v is not supported", l.Op))
		}
		expr, err := g.parseExpr(f, scope, l.X)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if reflect.TypeOf(expr).Kind() != reflect.Struct {
			return nil, wrap(f, n, trace.BadParameter("don't know how to take address of %v", reflect.TypeOf(expr).Kind()))
		}
		ptr := reflect.New(reflect.TypeOf(expr))
		ptr.Elem().Set(reflect.ValueOf(expr))
		return ptr.Interface(), nil
	case *ast.FuncLit:
		if l.Type.Results != nil && len(l.Type.Results.List) != 0 {
			return nil, wrap(f, n, trace.BadParameter("functions with return values are not supported"))
		}
		lambda := &force.LambdaFunction{
			Scope: force.WithLexicalScope(scope),
		}
		if l.Type.Params != nil && len(l.Type.Params.List) != 0 {
			for i, p := range l.Type.Params.List {
				if len(p.Names) != 1 {
					return nil, wrap(f, n, trace.BadParameter("lambda function parameter %v name is not supported", i))
				}
				arg, err := g.evalFunctionArg(f, lambda.Scope, p.Type)
				if err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
				param := force.LambdaParam{Name: p.Names[0].Name, Prototype: arg}
				lambda.Params = append(lambda.Params, param)
				if err := lambda.Scope.AddDefinition(param.Name, arg); err != nil {
					return nil, wrap(f, n, trace.Wrap(err))
				}
			}
		}
		// evaluate arguments within a new lexical scope
		nodes := make([]ast.Node, len(l.Body.List))
		for i := range l.Body.List {
			nodes[i] = l.Body.List[i]
		}
		var err error
		lambda.Statements, err = g.parseStatements(f, lambda.Scope, nodes)
		if err != nil {
			return nil, wrap(f, n, trace.Wrap(err))
		}
		return lambda, nil
	case *ast.ExprStmt:
		return g.parseExpr(f, scope, l.X)
	case *ast.SelectorExpr:
		fields := []force.String{force.String(l.Sel.Name)}
	accumulate:
		switch selector := l.X.(type) {
		case *ast.Ident:
			v, err := force.Var(scope)(force.String(selector.Name), fields...)
			if err != nil {
				return nil, wrap(f, selector, trace.Wrap(err))
			}
			return v, nil
		case *ast.SelectorExpr:
			l = selector
			fields = append([]force.String{force.String(selector.Sel.Name)}, fields...)
			goto accumulate
		default:
			return nil, wrap(f, n, trace.BadParameter("expected identifier, got %T for %v", l.X, l.X))
		}
	default:
		return nil, wrap(f, n, trace.BadParameter("%T is not supported", n))
	}
}

func (g *gParser) evalFunctionArg(f *token.FileSet, scope force.Group, n ast.Node) (interface{}, error) {
	switch l := n.(type) {
	case *ast.Ident:
		return literalZeroValue(l.Name)
	case *ast.ArrayType:
		arrayType, ok := l.Elt.(*ast.Ident)
		if !ok {
			return nil, wrap(f, n, trace.BadParameter("unsupported composite literal: %v %T", l.Elt, l.Elt))
		}
		switch arrayType.Name {
		case force.StringType:
			return force.StringSlice(nil), nil
		case force.IntType:
			return force.IntSlice(nil), nil
		case force.BoolType:
			return force.BoolSlice(nil), nil
		}
		return nil, wrap(f, n, trace.BadParameter("%T is not supported", n))
	case *ast.StructType:
		structFields := make([]reflect.StructField, len(l.Fields.List))
		for i := range l.Fields.List {
			field := l.Fields.List[i]
			fieldTypeI, ok := field.Type.(*ast.Ident)
			if !ok {
				return nil, wrap(f, field.Type, trace.BadParameter("expected identifier, got %v", field.Type))
			}
			fieldName := field.Names[0].Name
			if force.StartsWithLower(fieldName) {
				return nil, wrap(f, field.Type,
					trace.BadParameter("struct field %v has to start with upper case: %v", fieldName, force.Capitalize(fieldName)))
			}
			switch fieldTypeI.Name {
			case force.StringType:
				varType := reflect.TypeOf((*force.StringVar)(nil)).Elem()
				structFields[i] = reflect.StructField{
					Name: fieldName,
					Type: varType,
				}
			case force.BoolType:
				varType := reflect.TypeOf((*force.BoolVar)(nil)).Elem()
				structFields[i] = reflect.StructField{
					Name: fieldName,
					Type: varType,
				}
			case force.IntType:
				varType := reflect.TypeOf((*force.IntVar)(nil)).Elem()
				structFields[i] = reflect.StructField{
					Name: fieldName,
					Type: varType,
				}
			default:
				return nil, wrap(f, field.Type, trace.BadParameter("unsupported struct type %v, supported: string", field.Type))
			}
		}
		structProto := reflect.StructOf(structFields)
		return createStruct(structProto, nil, false)
	default:
		return nil, wrap(f, n, trace.BadParameter("%T is not supported", n))
	}
}

func (g *gParser) getFunction(scope force.Group, name string) (force.Function, error) {
	fnI := g.scope.Value(force.ContextKey(name))
	if fnI == nil {
		fnI, err := scope.GetDefinition(name)
		if err == nil {
			newFn, ok := fnI.(force.Function)
			if !ok {
				return nil, trace.BadParameter("expected Function, got %T", fnI)
			}
			return newFn, nil
		}
		return nil, trace.BadParameter("function %v is not defined", name)
	}
	fn, ok := fnI.(force.Function)
	if !ok {
		return nil, trace.BadParameter("function %v is not a variable", name)
	}
	return fn, nil
}

func getIdentifier(f *token.FileSet, node ast.Node) (string, error) {
	sexpr, ok := node.(*ast.SelectorExpr)
	if ok {
		id, ok := sexpr.X.(*ast.Ident)
		if !ok {
			return "", wrap(f, node, trace.BadParameter("expected selector identifier, got: %T in %#v", sexpr.X, sexpr.X))
		}
		return fmt.Sprintf("%s.%s", id.Name, sexpr.Sel.Name), nil
	}
	id, ok := node.(*ast.Ident)
	if !ok {
		return "", wrap(f, node, trace.BadParameter("expected identifier, got: %T", node))
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
	fnType := reflect.TypeOf(f)
	for i, a := range args {
		// variadic call
		var inType reflect.Type
		if i >= fnType.NumIn() {
			inType = fnType.In(fnType.NumIn() - 1)
		} else {
			inType = fnType.In(i)
		}
		converter, ok := reflect.Zero(inType).Interface().(force.Converter)
		if ok {
			out, err := converter.Convert(a)
			if err != nil {
				return nil, trace.Wrap(err)
			}
			arguments[i] = reflect.ValueOf(out)
		} else {
			arguments[i] = reflect.ValueOf(a)
		}
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

func createStruct(val interface{}, args map[string]interface{}, pointer bool) (v interface{}, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = trace.BadParameter("struct %v: %v %v %v", force.StructName(reflect.TypeOf(val)), r, val, args)
		}
	}()
	structType, ok := val.(reflect.Type)
	if !ok {
		return nil, trace.BadParameter("expected type, got %T", val)
	}
	st := reflect.New(structType)
	for i := 0; i < structType.NumField(); i++ {
		field := st.Elem().Field(i)
		iface, ok := field.Interface().(force.Converter)
		if ok {
			// This is something like *StringVar,
			// which means that it does not have to be initialized by default
			if field.Type().Kind() != reflect.Ptr {
				out, err := iface.Convert(force.Zero(field).Interface())
				if err != nil {
					return nil, trace.Wrap(err)
				}
				field.Set(reflect.ValueOf(out))
			}
		} else {
			field.Set(force.Zero(field))
		}
	}
	for key, val := range args {
		field, err := fieldByName(st.Elem(), key)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		if !field.CanSet() {
			return nil, trace.BadParameter("can't set value of %v", field)
		}
		iface, ok := field.Interface().(force.Converter)
		if ok {
			if field.Type().Kind() != reflect.Ptr {
				out, err := iface.Convert(val)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				field.Set(reflect.ValueOf(out))
			} else {
				new := reflect.New(field.Type().Elem())
				expr := new.Interface().(force.Converter)
				out, err := expr.Convert(val)
				if err != nil {
					return nil, trace.Wrap(err)
				}
				// out is not addressable, so this code
				// allocates the new version, assigns the value
				// and uses it's address
				allocCopy := reflect.New(reflect.TypeOf(out))
				allocCopy.Elem().Set(reflect.ValueOf(out))
				field.Set(allocCopy)
			}
		} else {
			field.Set(reflect.ValueOf(val))
		}
	}
	if pointer {
		return st.Interface(), nil
	}
	return st.Elem().Interface(), nil
}

func fieldTypeByName(st reflect.Type, name string) (*reflect.StructField, error) {
	for i := 0; i < st.NumField(); i++ {
		fieldType := st.Field(i)
		if fieldType.Type.Kind() == reflect.Struct && fieldType.Anonymous {
			f, err := fieldTypeByName(fieldType.Type, name)
			if err == nil {
				return f, nil
			}
			if !trace.IsNotFound(err) {
				return nil, trace.Wrap(err)
			}
		} else {
			if fieldType.Name == name {
				return &fieldType, nil
			}
		}
	}

	return nil, trace.NotFound("field type %v is not found in struct %v", name, force.StructName(st))
}

func fieldByName(st reflect.Value, name string) (reflect.Value, error) {
	stType := st.Type()
	for i := 0; i < st.NumField(); i++ {
		field := st.Field(i)
		fieldType := stType.Field(i)

		if fieldType.Type.Kind() == reflect.Struct && fieldType.Anonymous {
			f, err := fieldByName(field, name)
			if err == nil {
				return f, nil
			}
			if !trace.IsNotFound(err) {
				return reflect.Value{}, trace.Wrap(err)
			}
		} else {
			if fieldType.Name == name {
				return field, nil
			}
		}
	}

	return reflect.Value{}, trace.NotFound("field %v is not found in struct %v", name, force.StructName(st.Type()))
}

// wrap wraps parse error
func wrap(f *token.FileSet, n ast.Node, err error) error {
	if _, ok := trace.Unwrap(err).(*force.CodeError); ok {
		return err
	}
	codeErr := &force.CodeError{
		Snippet: force.Snippet{
			Pos: f.Position(n.Pos()),
		},
		Err: err,
	}
	// rewrap to preserve original stack
	terr, ok := err.(*trace.TraceErr)
	if !ok {
		return codeErr
	}
	codeErr.Err = trace.Unwrap(err)
	terr.Err = codeErr
	return terr
}
