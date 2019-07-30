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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/snapshots/overlay"

	"github.com/gravitational/force"

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

// Config specifies builder config
type Config struct {
	// Context
	Context context.Context
	// GlobalContext is a base directory path for overlayfs other types
	// of snapshotting
	GlobalContext string
	// Backend specifies build backend
	Backend string
	// SessionName is a default build session name
	SessionName string
	// Server is an optional registry server to login into
	Server string
	// Username is the registry username
	Username string
	// Secret is a registry secret
	Secret string
	// SecretFile is a path to secret
	SecretFile string
	// Insecure turns off security for image pull/push
	Insecure bool
	// Group is a builder plugin group
	Group force.Group
}

// CheckAndSetDefaults checks and sets default values
func (i *Config) CheckAndSetDefaults() error {
	if i.Context == nil {
		i.Context = context.TODO()
	}
	if i.Group == nil {
		return trace.BadParameter("missing parameter Group")
	}
	if i.GlobalContext == "" {
		baseDir := os.Getenv("HOME")
		if baseDir == "" {
			baseDir = os.TempDir()
		}
		i.GlobalContext = filepath.Join(baseDir, ".local", "share", "img")
	}
	if i.Backend == "" {
		err := overlay.Supported(i.GlobalContext)
		if err == nil {
			i.Backend = OverlayFSBackend
		} else {
			i.Backend = NativeBackend
			force.Debugf("Picking native backend, overlayfs is not supported: %v.", err)
		}
	}
	if i.SessionName == "" {
		i.SessionName = SessionName
	}
	if i.SecretFile != "" {
		data, err := ioutil.ReadFile(i.SecretFile)
		if err != nil {
			return trace.ConvertSystemError(err)
		}
		i.Secret = string(data)
	}
	return nil
}

// Image specifies docker image to build
type Image struct {
	// Context is a path or URL to the bulid context
	Context string
	// Dockerfile is a path or the URL to the dockerfile
	Dockerfile string
	// Tag is a tag in the spec of image:tag (optional tag part)
	Tag string
	// NoCache turns off caching
	NoCache bool
	// Platforms is a list of target platforms
	Platforms []string
	// Target is the target build stage to build
	Target string
	// Secrets is a list of secrets
	// mounted in the build container
	Secrets []Secret
	// Args is a list of the build arguments
	Args []Arg
}

// Secret is a secret passed to docker builds
type Secret struct {
	ID string
	// File is a path to a secret
	File force.StringVar
}

func (s *Secret) CheckAndSetDefaults() error {
	if s.ID == "" {
		return trace.BadParameter("missing ID value of the secret")
	}
	if s.File == nil {
		return trace.BadParameter("missing File of the secret %q", s.ID)
	}
	return nil
}

type Arg struct {
	// Key is a build argument key
	Key string
	// Val is a build argument value
	Val force.StringVar
}

func (a *Arg) CheckAndSetDefaults() error {
	if a.Key == "" {
		return trace.BadParameter("missing Key value of the build argument")
	}
	if a.Val == nil {
		return trace.BadParameter("missing Val value of the build argument %q", a.Key)
	}
	return nil
}

// CheckAndSetDefaults checks and sets default values
func (i *Image) CheckAndSetDefaults() error {
	if i.Tag == "" {
		return trace.BadParameter("specify image tag, e.g. 'example'")
	}
	_, err := reference.ParseNormalizedNamed(i.Tag)
	if err != nil {
		return trace.BadParameter("parsing image name %q failed: %v", i.Tag, err)
	}
	if i.Context == "" {
		i.Context = "."
	}
	if i.Dockerfile == "" {
		i.Dockerfile, err = securejoin.SecureJoin(i.Context, "Dockerfile")
		if err != nil {
			return trace.Wrap(err)
		}
	}
	if len(i.Platforms) == 0 {
		i.Platforms = []string{platforms.DefaultString()}
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
func New(cfg Config) (*Builder, error) {
	if err := cfg.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	if err := utils.RuncBinaryExists(cfg.Context); err != nil {
		return nil, trace.Wrap(err)
	}

	// Create the directory used for build snapshots
	root := filepath.Join(cfg.GlobalContext, RuncExecutor, cfg.Backend)
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, trace.Wrap(err)
	}

	sessManager, err := session.NewManager()
	if err != nil {
		return nil, trace.Wrap(err)
	}

	b := &Builder{
		Config:      cfg,
		sessManager: sessManager,
		root:        root,
	}
	// Create the worker opts.
	opt, err := b.createWorkerOpt(b.Context, true)
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
	Config
	logger      force.Logger
	sessManager *session.Manager
	root        string
	controller  *control.Controller
	opt         *base.WorkerOpt
}
