package force

import (
	"reflect"

	"github.com/gravitational/trace"
)

// EmptyContext returns empty execution context
func EmptyContext() ExecutionContext {
	return WithRuntimeScope(nil)
}

// ZeroFromAST returns zero value original struct from AST
func ZeroFromAST(in interface{}) (interface{}, error) {
	if in == nil {
		return nil, trace.BadParameter("input value can not be nil")
	}
	inVal := reflect.ValueOf(in)
	if inVal.Type().Kind() == reflect.Ptr {
		inVal = inVal.Elem()
	}
	if inVal.Type().Kind() != reflect.Struct {
		return nil, trace.BadParameter("EvalFromAST works with structs, got %v", inVal.Type().Kind())
	}
	origVal := inVal.FieldByName(metadataFieldName)
	if !origVal.IsValid() {
		return nil, trace.BadParameter("EvalFromAST expected struct converted into AST, got %T", in)
	}
	return reflect.Zero(origVal.Type()).Interface(), nil
}

// EvalFromAST returns a new instance of the original struct marshaled
// from converted object, returns error if the object was not converted
// into AST
func EvalFromAST(ctx ExecutionContext, in interface{}) (interface{}, error) {
	if in == nil {
		return nil, trace.BadParameter("input value can not be nil")
	}
	inVal := reflect.ValueOf(in)
	if inVal.Type().Kind() == reflect.Ptr {
		inVal = inVal.Elem()
	}
	if inVal.Type().Kind() != reflect.Struct {
		return nil, trace.BadParameter("EvalFromAST works with structs, got %v", inVal.Type().Kind())
	}
	origVal := inVal.FieldByName(metadataFieldName)
	if !origVal.IsValid() {
		return nil, trace.BadParameter("EvalFromAST expected struct converted into AST, got %T", in)
	}
	newVal := reflect.New(origVal.Type())
	if err := EvalInto(ctx, inVal.Interface(), newVal.Interface()); err != nil {
		return nil, trace.Wrap(err)
	}
	return newVal.Elem().Interface(), nil
}

// EvalInto evaluates variable in within the execution context
// into variable out
func EvalInto(ctx ExecutionContext, inRaw, out interface{}) error {
	if inRaw == nil {
		return nil
	}
	in, err := Eval(ctx, inRaw)
	if err != nil {
		return trace.Wrap(err)
	}
	if in == nil {
		return nil
	}
	inType := reflect.TypeOf(in)
	outType := reflect.TypeOf(out)
	if outType.Kind() != reflect.Ptr {
		return trace.BadParameter("out should be a pointer, got %T(%v)", out, outType.Kind())
	}
	outVal := reflect.ValueOf(out)

	switch inType.Kind() {
	case reflect.Struct:
		if outType.Elem().Kind() != reflect.Struct {
			return trace.BadParameter("in is %v then out should be pointer to struct, got %T", inType, out)
		}
		inVal := reflect.ValueOf(in)
		for i := 0; i < inType.NumField(); i++ {
			fieldVal := inVal.Field(i)
			fieldType := inType.Field(i)
			if fieldVal.Interface() == nil {
				continue
			}
			eval, err := Eval(ctx, fieldVal.Interface())
			if err != nil {
				return trace.Wrap(err)
			}
			if eval == nil {
				continue
			}
			if fieldType.Name == metadataFieldName || fieldType.Tag.Get(codeTag) == codeSkip {
				continue
			}
			outField := outVal.Elem().FieldByName(fieldType.Name)
			if !outField.IsValid() {
				return trace.NotFound("struct %T has no field %v", out, fieldType.Name)
			}
			// simple case, can assign two primitive evaluated types
			if outField.Type().AssignableTo(reflect.TypeOf(eval)) {
				outField.Set(reflect.ValueOf(eval))
			} else {
				evalType := reflect.TypeOf(eval)
				evalValue := reflect.ValueOf(eval)
				if evalType.Kind() == reflect.Ptr && !evalValue.Elem().IsValid() {
					continue
				}
				if evalType.Kind() == reflect.Ptr && evalType.Elem().Kind() == reflect.Struct {
					tempVal := reflect.New(OriginalType(evalType.Elem()))
					err := EvalInto(ctx, reflect.ValueOf(eval).Elem().Interface(), tempVal.Interface())
					if err != nil {
						return trace.Wrap(err)
					}
					outField.Set(tempVal)
				} else {
					if err := EvalInto(ctx, eval, outField.Addr().Interface()); err != nil {
						return trace.Wrap(err)
					}
				}
			}
		}
		return nil
	case reflect.Ptr:
		return trace.BadParameter("can't evaluate %v(%T) into %v(%T)", in, in, out, out)
	case reflect.Slice:
		inVal := reflect.ValueOf(in)
		outSlice := reflect.MakeSlice(outType.Elem(), inVal.Len(), inVal.Len())
		for i := 0; i < inVal.Len(); i++ {
			elem := inVal.Index(i).Interface()
			if err := EvalInto(ctx, elem, outSlice.Index(i).Addr().Interface()); err != nil {
				return trace.Wrap(err)
			}
		}
		outVal.Elem().Set(outSlice)
		return nil
	case reflect.Map:
		inVal := reflect.ValueOf(in)
		outMap := reflect.MakeMapWithSize(outType.Elem(), inVal.Len())
		for _, key := range inVal.MapKeys() {
			elem := inVal.MapIndex(key).Interface()
			evalVal, err := Eval(ctx, elem)
			if err != nil {
				return trace.Wrap(err)
			}
			if evalVal == nil {
				continue
			}
			evalKey, err := Eval(ctx, key.Interface())
			if err != nil {
				return trace.Wrap(err)
			}
			if evalKey == nil {
				continue
			}
			outMap.SetMapIndex(reflect.ValueOf(evalKey), reflect.ValueOf(evalVal))
		}
		outVal.Elem().Set(outMap)
		return nil
	default:
		evaluated, err := Eval(ctx, in)
		if err != nil {
			return trace.Wrap(err)
		}
		outElem := outVal.Elem()
		if !outElem.CanSet() {
			return trace.BadParameter("can't set value of %v(%T) to %v(%T)", out, out, evaluated, evaluated)
		}
		// a couple of heuristics to simplify assignment
		switch outElem.Interface().(type) {
		case *int64:
			switch e := evaluated.(type) {
			case int:
				evaluated = int64(e)
			}
		case *int32:
			switch e := evaluated.(type) {
			case int:
				evaluated = int32(e)
			}
		}
		evalVal := reflect.ValueOf(evaluated)
		evalType := evalVal.Type()
		// types are directly assignable
		if reflect.TypeOf(evaluated).AssignableTo(outElem.Type()) {
			outElem.Set(reflect.ValueOf(evaluated))
			return nil
		}
		// evaluated could be converted to the type
		if reflect.TypeOf(evaluated).ConvertibleTo(outElem.Type()) {
			converted := reflect.ValueOf(evaluated).Convert(outElem.Type())
			outElem.Set(converted)
			return nil
		}
		// target value is a pointer, and the original is not a zero
		if outElem.Kind() == reflect.Ptr {
			if evalVal.IsZero() {
				return nil
			}
			// this is a trick to set *bool to the bool on the right,
			// create a copy and assign address of it
			if outElem.Type() == reflect.PtrTo(evalVal.Type()) {
				tempVal := reflect.New(evalVal.Type())
				tempVal.Elem().Set(evalVal)
				outElem.Set(tempVal)
				return nil
			}
			// type is a pointer directly assignable
			if reflect.TypeOf(evaluated).AssignableTo(outElem.Type().Elem()) {
				tempVal := reflect.New(evalType)
				tempVal.Elem().Set(evalVal)
				outElem.Set(tempVal)
				return nil
			}
			// type is a pointer and evaluated could be converted to it
			if evalType.ConvertibleTo(outElem.Type().Elem()) {
				converted := evalVal.Convert(outElem.Type().Elem())
				tempVal := reflect.New(converted.Type())
				tempVal.Elem().Set(converted)
				outElem.Set(tempVal)
				return nil
			}
		}
		return trace.BadParameter("type %v is not convertible or assignable to %v", reflect.TypeOf(evaluated), outElem.Type())
	}
}

// Eval evaluates variable based on the execution context
func Eval(ctx ExecutionContext, variable interface{}) (interface{}, error) {
	if reflect.TypeOf(variable).Kind() == reflect.Ptr && reflect.ValueOf(variable).IsZero() {
		return nil, nil
	}
	switch v := variable.(type) {
	case []interface{}:
		outSlice := make([]interface{}, len(v))
		for i := range v {
			out, err := Eval(ctx, v[i])
			if err != nil {
				return nil, trace.Wrap(err)
			}
			outSlice[i] = out
		}
		return outSlice, nil
	case Expression:
		return v.Eval(ctx)
	default:
		return v, nil
	}
}

func PInt32(in int32) *int32 {
	return &in
}

func PInt64(in int64) *int64 {
	return &in
}
