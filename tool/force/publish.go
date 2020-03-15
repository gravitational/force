package main

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/gravitational/force/pkg/builder"
	"github.com/gravitational/force/pkg/runner"

	"github.com/gravitational/force"
	"github.com/gravitational/trace"
)

type publishConfig struct {
	program    string
	runcVer    string
	goVer      string
	builderCfg builder.Config
	repo       string
}

func publish(ctx context.Context, cfg publishConfig) error {
	runner.SetupInCLI(ctx,
		builder.Setup(cfg.builderCfg),
	).RunFunc(func(ctx force.ExecutionContext) error {
		return publishProgram(ctx, cfg)
	})
	return nil
}

// publishProgram is a utility function to build and publish pipeline spec
// accepts the path to file.go and target image path
// it assumes that it runs from force git root repository
func publishProgram(ctx force.ExecutionContext, cfg publishConfig) error {
	if filepath.IsAbs(cfg.program) {
		return trace.BadParameter("path to program should be relative to force repo, e.g. './examples/hello', got %q instead", cfg.program)
	}
	if !strings.HasPrefix(cfg.program, ".") {
		cfg.program = filepath.Join(".", cfg.program)
	}

	log := force.Log(ctx)

	defer builder.Prune(ctx)
	// Capture version
	if !strings.Contains(cfg.repo, ":") {
		ver, err := force.Command(ctx, "./build.assets/publish/version.sh")
		if err != nil {
			return trace.Wrap(err)
		}
		cfg.repo = cfg.repo + ":" + strings.TrimSpace(ver)
		log.Infof("Derived tag from git version: %v", cfg.repo)
	}

	force.Log(ctx).Infof("Going to build %v with go runtime %v, runc %v", cfg.repo, cfg.goVer, cfg.runcVer)

	// Build builds dockerfile and tags it in the local storage
	err := builder.Build(ctx, builder.Image{
		Dockerfile: "./build.assets/publish/Dockerfile",
		Context:    "./",
		Tag:        cfg.repo,
		Args: []builder.Arg{
			{Key: "GO_VER", Val: cfg.goVer},
			{Key: "RUNC_VER", Val: cfg.runcVer},
			{Key: "PROGRAM_PATH", Val: cfg.program},
		},
	})
	if err != nil {
		return trace.Wrap(err)
	}
	// Push the built image
	return trace.Wrap(builder.Push(ctx, builder.Image{Tag: cfg.repo}))
}
