package force

import (
	"fmt"
	"path/filepath"
	"reflect"
	"unicode"

	"github.com/gravitational/trace"
)

// Zero returns a zero value for interface
func Zero(iface reflect.Value) reflect.Value {
	switch i := iface.Interface().(type) {
	case Expression:
		// This is unclear - what if it's a legitimate pointer to some struct
		// not *StringVar
		if iface.Type().Kind() == reflect.Ptr && iface.IsZero() {
			new := reflect.New(iface.Type().Elem())
			expr := new.Interface().(Expression)
			return reflect.New(reflect.TypeOf(expr.Type()))
		}
		return reflect.Zero(reflect.TypeOf(i.Type()))
	default:
		return reflect.Zero(iface.Type())
	}
}

// ConvertValueToAST converts value to the value of the
// force-compatible type
func ConvertValueToAST(in interface{}) (interface{}, error) {
	if in == nil {
		return nil, trace.BadParameter("empty interface can't be converted")
	}
	switch val := in.(type) {
	case string:
		return String(val), nil
	case []string:
		out := make([]Expression, len(val))
		for i, v := range val {
			out[i] = String(v)
		}
		return Strings(out...)
	}
	return nil, trace.BadParameter(
		"don't know how to convert value of %T to interface", in)
}

// ConvertTypeToAST converts incoming type to the type understood
// by force interpreter
func ConvertTypeToAST(in reflect.Type) (reflect.Type, error) {
	return convertTypeToAST(in, []reflect.Type{in})
}

// types are used to detect self referrential types and avoid loops
func convertTypeToAST(in reflect.Type, types []reflect.Type) (reflect.Type, error) {
	switch in.Kind() {
	case reflect.Bool:
		return reflect.TypeOf(BoolVar{}), nil
	case reflect.Int:
		return reflect.TypeOf(IntVar{}), nil
	case reflect.Int64:
		return reflect.TypeOf(IntVar{}), nil
	case reflect.Int32:
		return reflect.TypeOf(IntVar{}), nil
	case reflect.String:
		return reflect.TypeOf(StringVar{}), nil
	case reflect.Ptr:
		out, err := convertTypeToAST(in.Elem(), append([]reflect.Type{in.Elem()}, types...))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		// this is to catch *bool that should be converted to BoolVar, not *BoolVar
		if out.Kind() == reflect.Interface {
			ifacePtr := reflect.New(out).Interface()
			switch ifacePtr.(type) {
			case *Expression:
				return out, nil
			}
		}
		return reflect.PtrTo(out), nil
	case reflect.Map:
		out, err := convertTypeToAST(in.Elem(), append([]reflect.Type{in.Elem()}, types...))
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return reflect.MapOf(in.Key(), out), nil
	case reflect.Slice:
		if in.Elem().Kind() == reflect.String {
			return reflect.TypeOf(StringsVar{}), nil
		}
		out, err := ConvertTypeToAST(in.Elem())
		if err != nil {
			return nil, trace.Wrap(err)
		}
		return reflect.SliceOf(out), nil
	case reflect.Struct:
		// in case of self referrential types,
		// just use interface{} as a placeholder
		// because golang StructOf does not support self referrential fields
		// convert the reference to interface{}
		if detectLoop(types) {
			return reflect.TypeOf((*interface{})(nil)).Elem(), nil
		}
		if _, ok := in.FieldByName(metadataFieldName); ok {
			// struct has been already converted
			return in, nil
		}
		fields := make([]reflect.StructField, 0, in.NumField())
		for i := 0; i < in.NumField(); i++ {
			name := in.Field(i).Name
			if !isExported(name) || in.Field(i).Tag.Get(codeTag) == codeSkip {
				continue
			}
			out, err := convertTypeToAST(in.Field(i).Type, append([]reflect.Type{in.Field(i).Type}, types...))
			if err != nil {
				return nil, trace.Wrap(err)
			}
			fields = append(fields, reflect.StructField{
				Name:      name,
				Type:      out,
				Anonymous: in.Field(i).Anonymous,
			})
		}
		metaField := reflect.StructField{
			Name: metadataFieldName,
			Type: in,
			Tag:  reflect.StructTag(fmt.Sprintf(`%v:%q`, codeTag, codeSkip)),
		}
		fields = append(fields, metaField)
		return reflect.StructOf(fields), nil
	case reflect.Interface:
		ifacePtr := reflect.New(in).Interface()
		switch ifacePtr.(type) {
		case *error:
			return in, nil
		case *Expression:
			return in, nil
		case *interface{}:
			return in, nil
		default:
			return reflect.TypeOf((*interface{})(nil)).Elem(), nil
		}
	default:
		return nil, trace.NotImplemented("type %v is not supported for type %v", in.Kind(), in)
	}
}

func detectLoop(in []reflect.Type) bool {
	if len(in) < 2 {
		return false
	}
	first := in[0]
	for _, v := range in[1:] {
		if v == first {
			return true
		}
	}
	return false
}

const (
	// metadataFieldName
	metadataFieldName = "ForceOrig"
	// codeTag
	codeTag  = "code"
	codeSkip = "-"
)

// ConvertFunctionToAST converts function fn into function with all
// arguments and return value converted to AST types
func ConvertFunctionToAST(fn interface{}) (Function, error) {
	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		return nil, trace.BadParameter("expected function got %v", fnType.Kind())
	}
	convertedOutTypes := make([]reflect.Type, fnType.NumOut())
	for i := 0; i < fnType.NumOut(); i++ {
		outType := fnType.Out(i)
		convertedOutType, err := ConvertTypeToAST(outType)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		convertedOutTypes[i] = convertedOutType
	}
	if len(convertedOutTypes) > 2 {
		return nil, trace.BadParameter("only support 1 or 2 return values (with last return type as error), got %v", len(convertedOutTypes))
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()
	if len(convertedOutTypes) > 1 {
		// in case of multiple return arguments, last one should be an error
		lastOutType := convertedOutTypes[len(convertedOutTypes)-1]
		if !lastOutType.AssignableTo(errType) {
			return nil, trace.BadParameter("expected last return type as error, got %v", lastOutType)
		}
	}

	convertedArgTypes := make([]reflect.Type, fnType.NumIn())
	for i := 0; i < fnType.NumIn(); i++ {
		argType := fnType.In(i)
		outArgType, err := ConvertTypeToAST(argType)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		convertedArgTypes[i] = outArgType
	}

	convertedLambda := func(args []reflect.Value) (results []reflect.Value) {
		out := &ConvertedFunc{
			calledWithArgs:    args,
			convertedOutTypes: convertedOutTypes,
			origFn:            fn,
			origFnType:        fnType,
		}
		return []reflect.Value{reflect.ValueOf(out)}
	}
	var out *ConvertedFunc
	convertedOut := []reflect.Type{reflect.TypeOf(out)}
	// this takes function function func(int) error -> func(IntVar) *ConvertedFunc
	convertedFuncType := reflect.FuncOf(convertedArgTypes, convertedOut, fnType.IsVariadic())
	convertedFunc := reflect.MakeFunc(convertedFuncType, convertedLambda)
	return &NopScope{Func: convertedFunc.Interface()}, nil
}

// ConvertedFunc holds converted function
type ConvertedFunc struct {
	calledWithArgs    []reflect.Value
	convertedOutTypes []reflect.Type
	origFn            interface{}
	origFnType        reflect.Type
}

// Eval evaluates function and returns string
func (c *ConvertedFunc) Eval(ctx ExecutionContext) (interface{}, error) {
	// evaluate all passed arguments
	args := make([]interface{}, len(c.calledWithArgs))
	for i := range c.calledWithArgs {
		out, err := Eval(ctx, c.calledWithArgs[i].Interface())
		if err != nil {
			return "", trace.Wrap(err)
		}
		args[i] = out
	}
	vals := make([]reflect.Value, len(args))
	for i := range args {
		vals[i] = reflect.ValueOf(args[i])
	}
	var returnValues []reflect.Value
	if c.origFnType.IsVariadic() {
		returnValues = reflect.ValueOf(c.origFn).CallSlice(vals)
	} else {
		returnValues = reflect.ValueOf(c.origFn).Call(vals)
	}
	switch len(returnValues) {
	case 1:
		return returnValues[0].Interface(), nil
	case 2:
		first := returnValues[0].Interface()
		second := returnValues[1].Interface()
		if second == nil {
			return first, nil
		}
		return first, second.(error)
	default:
		return "", trace.BadParameter("expected one or two return values, got %v", len(returnValues))
	}
}

func (c *ConvertedFunc) Type() interface{} {
	outType := c.convertedOutTypes[0]
	out := reflect.Zero(outType).Interface()
	if expr, ok := out.(Expression); ok {
		return expr.Type()
	}
	return outType
}

// MarshalCode marshals code
func (c *ConvertedFunc) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	return marshalGenFuncCode(ctx, c.origFn, c.origFnType, c.calledWithArgs)
}

func marshalGenFuncCode(ctx ExecutionContext, origFn interface{}, origFnType reflect.Type, calledWithArgs []reflect.Value) ([]byte, error) {
	ifaces := make([]interface{}, len(calledWithArgs))
	for i, a := range calledWithArgs {
		ifaces[i] = a.Interface()
	}
	// expand last argument in case of variadic functions
	if origFnType.IsVariadic() && len(calledWithArgs) > 0 {
		lastArg := calledWithArgs[len(calledWithArgs)-1]
		if lastArg.Kind() != reflect.Slice {
			return nil, trace.BadParameter("expected slice")
		}
		variadicIfaces := make([]interface{}, lastArg.Len())
		for i := 0; i < lastArg.Len(); i++ {
			variadicIfaces[i] = lastArg.Index(i).Interface()
		}
		ifaces = append(ifaces[:len(ifaces)-1], variadicIfaces...)
	}
	call := &FnCall{
		Package: filepath.Base(origFnType.PkgPath()),
		FnName:  FunctionName(origFn),
		Args:    ifaces,
	}
	return call.MarshalCode(ctx)
}

func isExported(name string) bool {
	for _, r := range name {
		return unicode.IsUpper(r)
	}
	return false
}
