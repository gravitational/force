package builder

import (
	"context"
	"io"
	"path/filepath"
	"strings"

	"github.com/gravitational/force"

	"github.com/containerd/console"
	"github.com/containerd/containerd/namespaces"
	controlapi "github.com/moby/buildkit/api/services/control"
	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/identity"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/filesync"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/session/testutil"
	"github.com/moby/buildkit/util/progress/progressui"

	"github.com/gravitational/trace"
	"golang.org/x/sync/errgroup"
)

// Run starts build
func (b *Builder) Run(ectx force.ExecutionContext, img Image) error {
	if err := img.CheckAndSetDefaults(ectx); err != nil {
		return trace.Wrap(err)
	}

	log := force.Log(ectx)
	tag, err := img.Tag.Eval(ectx)
	if err != nil {
		return trace.Wrap(err)
	}
	dockerfilePath, err := img.Dockerfile.Eval(ectx)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Building image %v, dockerfile %v.", tag, dockerfilePath)

	// create and execute a build session
	frontendAttrs := map[string]string{
		// We use the base for filename here because we already set up the
		// local dirs which sets the path in createController.
		"filename": filepath.Base(dockerfilePath),
		"target":   img.Target,
		"platform": strings.Join(img.Platforms, ","),
	}
	if img.NoCache {
		frontendAttrs["no-cache"] = ""
	}

	// Get the build args and add them to frontend attrs.
	for _, a := range img.Args {
		key, err := a.Key.Eval(ectx)
		if err != nil {
			return trace.Wrap(err)
		}
		val, err := a.Val.Eval(ectx)
		if err != nil {
			return trace.Wrap(err)
		}
		frontendAttrs["build-arg:"+key] = val
	}

	sess, sessDialer, err := b.Session(ectx, img)
	if err != nil {
		return trace.Wrap(err, "failed to create session")
	}

	id := identity.NewID()
	ctx := session.NewContext(ectx, sess.ID())
	ctx = namespaces.WithNamespace(ctx, "buildkit")
	eg, ctx := errgroup.WithContext(ctx)

	statusC := make(chan *controlapi.StatusResponse)
	eg.Go(func() error {
		return sess.Run(ctx, sessDialer)
	})
	// Solve the dockerfile.
	eg.Go(func() error {
		defer sess.Close()
		return b.solve(ctx, &controlapi.SolveRequest{
			Ref:      id,
			Session:  sess.ID(),
			Exporter: "image",
			ExporterAttrs: map[string]string{
				// in the future will be multiple tags
				"name": strings.Join([]string{tag}, ","),
			},
			Frontend:      FrontendDockerfile,
			FrontendAttrs: frontendAttrs,
		}, statusC)
	})
	writer := force.Writer(log)
	defer writer.Close()
	eg.Go(func() error {
		return showProgress(ctx, statusC, writer)
	})
	if err := eg.Wait(); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Successfully built %v.", tag)

	return nil
}

// Session creates the session manager and returns the session and it's
// dialer.
func (b *Builder) Session(ctx force.ExecutionContext, img Image) (*session.Session, session.Dialer, error) {
	sess, err := session.NewSession(ctx, string(b.cfg.sessionName), "")
	if err != nil {
		return nil, nil, trace.Wrap(err, "failed to create session")
	}

	contextPath, err := img.Context.Eval(ctx)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	dockerfilePath, err := img.Dockerfile.Eval(ctx)
	if err != nil {
		return nil, nil, trace.Wrap(err)
	}
	var syncedDirs []filesync.SyncedDir
	for name, d := range map[string]string{
		"context":    contextPath,
		"dockerfile": filepath.Dir(dockerfilePath),
	} {
		syncedDirs = append(syncedDirs, filesync.SyncedDir{Name: name, Dir: d})
	}
	sess.Allow(filesync.NewFSSyncProvider(syncedDirs))
	// Allow itself as auth provider
	// before it was sess.Allow(authprovider.NewDockerAuthProvider())
	sess.Allow(b)

	// Allow secrets! This was usually a big pain point in all docker
	// builds, because the context was a part of the image, so this is exciting
	if len(img.Secrets) > 0 {
		files := make([]secretsprovider.FileSource, len(img.Secrets))
		for i, s := range img.Secrets {
			id, err := s.ID.Eval(ctx)
			if err != nil {
				return nil, nil, trace.Wrap(err)
			}
			file, err := s.File.Eval(ctx)
			if err != nil {
				return nil, nil, trace.Wrap(err)
			}
			files[i] = secretsprovider.FileSource{ID: id, FilePath: file}
		}
		secretStore, err := secretsprovider.NewFileStore(files)
		if err != nil {
			return nil, nil, trace.Wrap(err)
		}
		sess.Allow(secretsprovider.NewSecretProvider(secretStore))
	}
	sessDialer := session.Dialer(
		testutil.TestStream(testutil.Handler(b.sessManager.HandleConn)))

	return sess, sessDialer, nil
}

func showProgress(ctx context.Context, ch chan *controlapi.StatusResponse, writer io.Writer) error {
	displayCh := make(chan *bkclient.SolveStatus)
	go func() {
		for resp := range ch {
			s := bkclient.SolveStatus{}
			for _, v := range resp.Vertexes {
				s.Vertexes = append(s.Vertexes, &bkclient.Vertex{
					Digest:    v.Digest,
					Inputs:    v.Inputs,
					Name:      v.Name,
					Started:   v.Started,
					Completed: v.Completed,
					Error:     v.Error,
					Cached:    v.Cached,
				})
			}
			for _, v := range resp.Statuses {
				s.Statuses = append(s.Statuses, &bkclient.VertexStatus{
					ID:        v.ID,
					Vertex:    v.Vertex,
					Name:      v.Name,
					Total:     v.Total,
					Current:   v.Current,
					Timestamp: v.Timestamp,
					Started:   v.Started,
					Completed: v.Completed,
				})
			}
			for _, v := range resp.Logs {
				s.Logs = append(s.Logs, &bkclient.VertexLog{
					Vertex:    v.Vertex,
					Stream:    int(v.Stream),
					Data:      v.Msg,
					Timestamp: v.Timestamp,
				})
			}
			displayCh <- &s
		}
		close(displayCh)
	}()
	var c console.Console
	return progressui.DisplaySolveStatus(ctx, "", c, writer, displayCh)
}
