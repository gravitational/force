package force

import (
	"github.com/gravitational/trace"
)

// Contains returns true if a sequence contains value
func Contains(iValue, iItem interface{}) (BoolVar, error) {
	switch val := iValue.(type) {
	case StringsVar:
		item, ok := iItem.(StringVar)
		if !ok {
			return nil, trace.BadParameter("Contains: second parameter should be string value")
		}
		return &ContainsStringAction{
			value: val,
			item:  item,
		}, nil
	default:
		return nil, trace.BadParameter("Contains: %T is not supported, supported matches on strings", iValue)
	}
}

type ContainsStringAction struct {
	value StringsVar
	item  StringVar
}

func (c *ContainsStringAction) Eval(ctx ExecutionContext) (bool, error) {
	values, err := c.value.Eval(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}
	item, err := c.item.Eval(ctx)
	if err != nil {
		return false, trace.Wrap(err)
	}
	for _, val := range values {
		if val == item {
			return true, nil
		}
	}
	return false, nil
}
