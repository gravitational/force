package main

import (
	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/runner"
)

func main() {
	runner.RunFunc(func(ctx force.ExecutionContext) error {
		_, err := force.Command(ctx, `echo "Hello, world!"`)
		return err
	})
}
