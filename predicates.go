package force

import (
	"reflect"

	"github.com/gravitational/trace"
)

// Contains returns true if a sequence contains value
func Contains(slice, item Expression) (Expression, error) {
	if reflect.TypeOf(slice.Type()).Kind() != reflect.Slice {
		return nil, trace.BadParameter("first argument should be slice, got %v", reflect.TypeOf(slice.Type()))
	}
	if reflect.TypeOf(slice.Type()).Elem() != reflect.TypeOf(item.Type()) {
		return nil, trace.BadParameter("slice and item types mismatch %v vs %v", reflect.TypeOf(slice.Type()).Elem(), reflect.TypeOf(item.Type()))
	}
	return &ContainsAction{
		slice: slice,
		item:  item,
	}, nil
}

type ContainsAction struct {
	slice Expression
	item  Expression
}

// MarshalCode marshals the variable to code representation
func (c *ContainsAction) MarshalCode(ctx ExecutionContext) ([]byte, error) {
	call := &FnCall{
		Fn:   Contains,
		Args: []interface{}{c.slice, c.item},
	}
	return call.MarshalCode(ctx)
}

func (c *ContainsAction) Type() interface{} {
	return false
}

func (c *ContainsAction) Eval(ctx ExecutionContext) (interface{}, error) {
	values, err := c.slice.Eval(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}
	item, err := c.item.Eval(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}
	valValues := reflect.ValueOf(values)
	for i := 0; i < valValues.Len(); i++ {
		val := valValues.Index(i).Interface()
		if val == item {
			return true, nil
		}
	}
	return false, nil
}
