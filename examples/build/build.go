package main

import (
	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/runner"

	_ "github.com/gravitational/force/internal/unshare"
)

func main() {
	runner.Reexec()
	runner.Setup(
		builder.Setup(builder.Config{}),
	).RunFunc(func(ctx force.ExecutionContext) error {
		defer builder.Prune(ctx)
		return builder.Build(ctx, builder.Image{Tag: `example`})
	})
}
