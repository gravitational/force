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
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/diff/apply"
	"github.com/containerd/containerd/diff/walking"
	ctdmetadata "github.com/containerd/containerd/metadata"
	"github.com/containerd/containerd/platforms"
	ctdsnapshot "github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/native"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/moby/buildkit/cache/metadata"
	"github.com/moby/buildkit/executor"
	executoroci "github.com/moby/buildkit/executor/oci"
	"github.com/moby/buildkit/executor/runcexecutor"
	containerdsnapshot "github.com/moby/buildkit/snapshot/containerd"
	"github.com/moby/buildkit/util/binfmt_misc"
	"github.com/moby/buildkit/util/leaseutil"
	"github.com/moby/buildkit/util/network"
	"github.com/moby/buildkit/util/resolver"
	"github.com/moby/buildkit/util/throttle"
	"github.com/moby/buildkit/worker/base"
	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/opencontainers/runc/libcontainer/system"
	"github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"

	"github.com/gravitational/trace"
)

// createWorkerOpt creates a base.WorkerOpt to be used for a new worker.
func (b *Builder) createWorkerOpt(ctx context.Context, withExecutor bool) (opt base.WorkerOpt, err error) {
	// Create the metadata store.
	md, err := metadata.NewStore(filepath.Join(b.root, "metadata.db"))
	if err != nil {
		return opt, err
	}
	snapshotRoot := filepath.Join(b.root, "snapshots")
	unprivileged := system.GetParentNSeuid() != 0

	// Create the snapshotter.
	var snapshotter ctdsnapshot.Snapshotter
	switch b.Backend {
	case NativeBackend:
		snapshotter, err = native.NewSnapshotter(snapshotRoot)
	case OverlayFSBackend:
		// On some distros such as Ubuntu overlayfs can be mounted without privileges
		snapshotter, err = overlay.NewSnapshotter(snapshotRoot)
	default:
		// "auto" backend needs to be already resolved on Client instantiation
		return opt, trace.BadParameter("%s is not a valid snapshots backend", b.Backend)
	}
	if err != nil {
		return opt, trace.BadParameter("creating %s snapshotter failed: %v", b.Backend, err)
	}

	var exe executor.Executor
	if withExecutor {
		exeOpt := runcexecutor.Opt{
			Root:        filepath.Join(b.root, "executor"),
			Rootless:    unprivileged,
			ProcessMode: processMode(),
		}
		// generates a proviers set with host network used by default
		providers, err := network.Providers(network.Opt{Mode: "host"})
		if err != nil {
			return opt, trace.Wrap(err)
		}
		exe, err = runcexecutor.New(exeOpt, providers)
		if err != nil {
			return opt, trace.Wrap(err)
		}
	}

	// Create the content store locally.
	contentStore, err := local.NewStore(filepath.Join(b.root, "content"))
	if err != nil {
		return opt, trace.Wrap(err)
	}

	// Open the bolt database for metadata.
	db, err := bolt.Open(filepath.Join(b.root, "containerdmeta.db"), 0644, nil)
	if err != nil {
		return opt, trace.Wrap(err)
	}

	// Create the new database for metadata.
	mdb := ctdmetadata.NewDB(db, contentStore, map[string]ctdsnapshot.Snapshotter{
		b.Backend: snapshotter,
	})
	if err := mdb.Init(ctx); err != nil {
		return opt, trace.Wrap(err)
	}

	// Create the image store.
	imageStore := ctdmetadata.NewImageStore(mdb)

	// Create the garbage collector.
	throttledGC := throttle.Throttle(time.Second, func() {
		if _, err := mdb.GarbageCollect(context.TODO()); err != nil {
			logrus.Errorf("GC error: %+v", err)
		}
	})

	gc := func(ctx context.Context) error {
		throttledGC()
		return nil
	}

	contentStore = containerdsnapshot.NewContentStore(mdb.ContentStore(), "buildkit", gc)

	id, err := base.ID(b.root)
	if err != nil {
		return opt, err
	}

	xlabels := base.Labels("oci", b.Backend)

	var supportedPlatforms []specs.Platform
	for _, p := range binfmt_misc.SupportedPlatforms() {
		parsed, err := platforms.Parse(p)
		if err != nil {
			return opt, err
		}
		supportedPlatforms = append(supportedPlatforms, platforms.Normalize(parsed))
	}

	opt = base.WorkerOpt{
		//
		ID:            id,
		Labels:        xlabels,
		MetadataStore: md,
		Executor:      exe,
		Snapshotter: containerdsnapshot.NewSnapshotter(
			b.Backend, mdb.Snapshotter(b.Backend), contentStore, md, "buildkit", gc, nil),
		ContentStore:       contentStore,
		Applier:            apply.NewFileSystemApplier(contentStore),
		Differ:             walking.NewWalkingDiff(contentStore),
		ImageStore:         imageStore,
		Platforms:          supportedPlatforms,
		ResolveOptionsFunc: resolver.NewResolveOptionsFunc(nil),
		LeaseManager:       leaseutil.WithNamespace(leaseutil.NewManager(mdb), "buildkit"),
	}

	return opt, err
}

func processMode() executoroci.ProcessMode {
	mountArgs := []string{"-t", "proc", "none", "/proc"}
	cmd := exec.Command("mount", mountArgs...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Pdeathsig:    syscall.SIGKILL,
		Cloneflags:   syscall.CLONE_NEWPID,
		Unshareflags: syscall.CLONE_NEWNS,
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		logrus.Warnf("Process sandbox is not available, consider unmasking procfs: %v, err: %v", string(b), err)
		return executoroci.NoProcessSandbox
	}
	return executoroci.ProcessSandbox
}
