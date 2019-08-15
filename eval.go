package force

import (
	"fmt"

	"github.com/gravitational/trace"
)

// Eval evaluates variable based on the execution context
func Eval(ctx ExecutionContext, variable interface{}) (interface{}, error) {
	switch v := variable.(type) {
	case IntVar:
		return v.Eval(ctx)
	case BoolVar:
		return v.Eval(ctx)
	case StringVar:
		return v.Eval(ctx)
	default:
		return nil, trace.BadParameter("unsupported value %T", v)
	}
}

// Sprintf is just like Sprintf
func Sprintf(format String, args ...interface{}) StringVar {
	return StringVarFunc(func(ctx ExecutionContext) (string, error) {
		eval := make([]interface{}, len(args))
		var err error
		for i := range args {
			eval[i], err = Eval(ctx, args[i])
			if err != nil {
				return "", trace.Wrap(err)
			}
		}
		return fmt.Sprintf(string(format), eval...), nil
	})
}

// EvalString evaluates empty or missing string into ""
func EvalString(ctx ExecutionContext, v StringVar) (string, error) {
	if v == nil {
		return "", nil
	}
	return v.Eval(ctx)
}

// EvalBool evaluates empty or unspecified bool to false
func EvalBool(ctx ExecutionContext, in BoolVar) (bool, error) {
	if in == nil {
		return false, nil
	}
	return in.Eval(ctx)
}

func EvalPInt64(ctx ExecutionContext, in IntVar) (*int64, error) {
	if in == nil {
		return nil, nil
	}
	out, err := in.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	val := int64(out)
	return &val, nil
}

func EvalPInt32(ctx ExecutionContext, in IntVar) (*int32, error) {
	if in == nil {
		return nil, nil
	}
	out, err := in.Eval(ctx)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	val := int32(out)
	return &val, nil
}

func PInt32(in int32) *int32 {
	return &in
}
