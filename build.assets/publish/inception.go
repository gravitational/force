package main

import (
	"fmt"
	"strings"

	"github.com/gravitational/force"
	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/runner"

	_ "github.com/gravitational/force/internal/unshare"
	"github.com/gravitational/trace"
)

func main() {
	runner.Reexec()

	runner.Setup(
		// Builder configures docker builder
		builder.Setup(builder.Config{
			// Logs into GCR server
			Server: "gcr.io",
			// Username is a username to login with the registry server
			Username: runner.ExitWithoutEnv("REGISTRY_USERNAME"),
			// SecretFile is a registry password
			SecretFile: runner.ExitWithoutEnv("REGISTRY_SECRET"),
		}),
	).RunFunc(func(ctx force.ExecutionContext) error {
		defer builder.Prune(ctx)
		// Capture version
		ver, err := force.Command(ctx, "./version.sh")
		if err != nil {
			return trace.Wrap(err)
		}
		ver = strings.TrimSpace(ver)

		// Image is an image name to build
		image := fmt.Sprintf(`gcr.io/kubeadm-167321/force:%v`, ver)
		// Go version to use for builds
		goVer := "1.13.1"
		// Runc version to include
		runcVer := "1.0.0-rc8"
		force.Log(ctx).Infof("Going to build %v with go runtime %v, runc %v", image, goVer, runcVer)

		// Build builds dockerfile and tags it in the local storage
		err = builder.Build(ctx, builder.Image{
			Dockerfile: "./Dockerfile",
			Context:    "../",
			Tag:        image,
			Args:       []builder.Arg{{Key: "GO_VER", Val: goVer}, {Key: "RUNC_VER", Val: runcVer}},
		})
		if err != nil {
			return trace.Wrap(err)
		}
		// Push the built image
		return trace.Wrap(builder.Push(ctx, builder.Image{Tag: image}))
	})
}
