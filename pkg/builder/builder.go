/*
The MIT License (MIT)

Copyright (c) 2018 The Genuinetools Authors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package builder

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/gravitational/force"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots/overlay"
	securejoin "github.com/cyphar/filepath-securejoin"
	"github.com/docker/distribution/reference"
	"github.com/gravitational/force/internal/utils"
	"github.com/moby/buildkit/control"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/worker/base"

	"github.com/gravitational/trace"
)

const (
	// NativeBackend defines the native backend.
	NativeBackend = "native"
	// OverlayFSBackend defines the overlayfs backend.
	OverlayFSBackend = "overlayfs"
	// RuncExecutor is a name of runc directory executor
	RuncExecutor = "runc"
	// SessionName is a default session name
	SessionName = "img"
	// FrontendDockerfile is a name of the buildkit frontend
	FrontendDockerfile = "dockerfile.v0"
)

// BuilderConfig specifies builder config
type BuilderConfig struct {
	// Context is an execution context for the plugin setup
	Context force.ExecutionContext `code:"-"`
	// Group is a process group that this plugin sets up for
	Group force.Group `code:"-"`
	// GlobalContext is a base directory path for overlayfs other types
	// of snapshotting
	GlobalContext force.StringVar
	// Backend specifies build backend
	Backend force.StringVar
	// SessionName is a default build session name
	SessionName force.StringVar
	// Server is an optional registry server to login into
	Server force.StringVar
	// Username is the registry username
	Username force.StringVar
	// Secret is a registry secret
	Secret force.StringVar
	// SecretFile is a path to registry secret file
	SecretFile force.StringVar
	// Insecure turns off security for image pull/push
	Insecure force.BoolVar
}

// evaluatedConfig contains evaluated configuration parameters
type evaluatedConfig struct {
	group         force.Group
	context       force.ExecutionContext
	server        string
	username      string
	secret        string
	globalContext string
	backend       string
	sessionName   string
	insecure      bool
}

// CheckAndSetDefaults checks and sets default values
func (i *BuilderConfig) CheckAndSetDefaults(ctx force.ExecutionContext) (*evaluatedConfig, error) {
	if i.Context == nil {
		return nil, trace.BadParameter("missing parameter Context")
	}
	if i.Group == nil {
		return nil, trace.BadParameter("missing parameter Group")
	}
	cfg := evaluatedConfig{
		group:   i.Group,
		context: i.Context,
	}
	var err error
	cfg.globalContext, err = force.EvalString(ctx, i.GlobalContext)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.globalContext == "" {
		baseDir := os.Getenv("HOME")
		if baseDir == "" {
			baseDir = os.TempDir()
		}
		cfg.globalContext = filepath.Join(baseDir, ".local", "share", "img")
	}
	cfg.backend, err = force.EvalString(ctx, i.GlobalContext)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.backend == "" {
		err := overlay.Supported(cfg.globalContext)
		if err == nil {
			cfg.backend = OverlayFSBackend
		} else {
			cfg.backend = NativeBackend
			force.Debugf("Picking native backend, overlayfs is not supported: %v.", err)
		}
	}
	if cfg.sessionName, err = force.EvalString(ctx, i.SessionName); err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.sessionName == "" {
		i.SessionName = force.String(SessionName)
	}
	if cfg.username, err = force.EvalString(ctx, i.Username); err != nil {
		return nil, trace.Wrap(err)
	}
	secretFile, err := force.EvalString(ctx, i.SecretFile)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if secretFile != "" {
		data, err := ioutil.ReadFile(secretFile)
		if err != nil {
			return nil, trace.ConvertSystemError(err)
		}
		i.Secret = force.String(data)
	}
	if cfg.secret, err = force.EvalString(ctx, i.Secret); err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.server, err = force.EvalString(ctx, i.Server); err != nil {
		return nil, trace.Wrap(err)
	}
	if cfg.insecure, err = force.EvalBool(ctx, i.Insecure); err != nil {
		return nil, trace.Wrap(err)
	}
	return &cfg, nil
}

// Image specifies docker image to build
type Image struct {
	// Context is a path or URL to the bulid context
	Context force.StringVar
	// Dockerfile is a path or the URL to the dockerfile
	Dockerfile force.StringVar
	// Tag is a tag in the spec of image:tag (optional tag part)
	Tag force.StringVar
	// NoCache turns off caching
	NoCache force.BoolVar
	// Platforms is a list of target platforms
	Platforms []force.StringVar
	// Target is the target build stage to build
	Target force.StringVar
	// Secrets is a list of secrets
	// mounted in the build container
	Secrets []Secret
	// Args is a list of the build arguments
	Args []Arg
}

// Secret is a secret passed to docker builds
type Secret struct {
	// ID is a secret ID
	ID force.StringVar
	// File is a path to a secret
	File force.StringVar
}

func (s *Secret) CheckAndSetDefaults() error {
	if s.ID == nil {
		return trace.BadParameter("missing ID value of the secret")
	}
	if s.File == nil {
		return trace.BadParameter("missing File of the secret %q", s.ID)
	}
	return nil
}

type Arg struct {
	// Key is a build argument key
	Key force.StringVar
	// Val is a build argument value
	Val force.StringVar
}

func (a *Arg) CheckAndSetDefaults() error {
	if a.Key == nil {
		return trace.BadParameter("missing Key value of the build argument")
	}
	if a.Val == nil {
		return trace.BadParameter("missing Val value of the build argument %q", a.Key)
	}
	return nil
}

const (
	// CurrentDir is a notation for current dir
	CurrentDir = "."
	// Dockerfile is a standard dockerfile name
	Dockerfile = "Dockerfile"
)

// CheckAndSetDefaults checks and sets default values
func (i *Image) CheckAndSetDefaults(ctx force.ExecutionContext) error {
	tagName, err := force.EvalString(ctx, i.Tag)
	if err != nil {
		return trace.Wrap(err)
	}
	if tagName == "" {
		return trace.BadParameter("specify image tag, e.g. 'example'")
	}
	_, err = reference.ParseNormalizedNamed(tagName)
	if err != nil {
		return trace.BadParameter("parsing image name %q failed: %v", tagName, err)
	}
	if i.Context == nil {
		i.Context = force.String(CurrentDir)
	}
	contextPath, err := i.Context.Eval(ctx)
	if err != nil {
		return trace.Wrap(err)
	}
	if contextPath == "" {
		i.Context = force.String(CurrentDir)
	}
	dockerfilePath, err := force.EvalString(ctx, i.Dockerfile)
	if err != nil {
		return trace.Wrap(err)
	}
	if dockerfilePath == "" {
		dockerfilePath, err := securejoin.SecureJoin(contextPath, Dockerfile)
		if err != nil {
			return trace.Wrap(err)
		}
		i.Dockerfile = force.String(dockerfilePath)
	}

	if len(i.Platforms) == 0 {
		i.Platforms = []force.StringVar{force.String(platforms.DefaultString())}
	}
	for _, s := range i.Secrets {
		if err := s.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	for _, a := range i.Args {
		if err := a.CheckAndSetDefaults(); err != nil {
			return trace.Wrap(err)
		}
	}
	return nil
}

func (i Image) String() string {
	return fmt.Sprintf("image tag %v, dockerfile %v", i.Tag, i.Dockerfile)
}

// New returns a new builder
func New(cfg BuilderConfig) (*Builder, error) {
	evalCfg, err := cfg.CheckAndSetDefaults(nil)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if err := utils.RuncBinaryExists(cfg.Context); err != nil {
		return nil, trace.Wrap(err)
	}

	// Create the directory used for build snapshots
	root := filepath.Join(string(evalCfg.globalContext), RuncExecutor, string(evalCfg.backend))
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, trace.Wrap(err)
	}

	sessManager, err := session.NewManager()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	b := &Builder{
		cfg:         *evalCfg,
		sessManager: sessManager,
		root:        root,
	}
	// Create the worker opts.
	opt, err := b.createWorkerOpt(b.cfg.context, true)
	if err != nil {
		return nil, trace.Wrap(err, "creating worker opt failed")
	}
	controller, err := b.createController(&opt)
	if err != nil {
		// TODO: cleanup resources e.g. opt close?
		return nil, trace.Wrap(err)
	}
	b.controller = controller
	b.opt = &opt
	return b, nil
}

// Builder is a new container image builder
type Builder struct {
	// cfg is evaluated config
	cfg         evaluatedConfig
	logger      force.Logger
	sessManager *session.Manager
	root        string
	controller  *control.Controller
	opt         *base.WorkerOpt
}
