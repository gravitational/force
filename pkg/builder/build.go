package builder

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
)

func Build(img Image) (force.Action, error) {
	if err := img.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &BuildAction{
		Image: img,
	}, nil
}

type BuildAction struct {
	Image Image
}

func (b *BuildAction) Run(ctx force.ExecutionContext) (force.ExecutionContext, error) {
	builder, err := New(Config{
		Context: ctx.Context(),
	})
	if err != nil {
		return nil, trace.Wrap(err)
	}
	return ctx, builder.Run(ctx.Context(), b.Image)
}
