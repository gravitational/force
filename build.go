package force

import (
	"context"

	"github.com/gravitational/force/pkg/builder"

	"github.com/gravitational/trace"
)

func Build(img builder.Image) (Action, error) {
	if err := img.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &BuildAction{
		Image: img,
	}, nil
}

type BuildAction struct {
	Image builder.Image
}

func (b *BuildAction) Run(ctx context.Context) error {
	builder, err := builder.New(builder.Config{
		Context: ctx,
	})
	if err != nil {
		return trace.Wrap(err)
	}
	return builder.Run(ctx, b.Image)
}
