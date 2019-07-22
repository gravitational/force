package builder

import (
	"context"
	"fmt"

	"github.com/gravitational/force"

	"github.com/containerd/containerd/namespaces"
	"github.com/docker/distribution/reference"
	"github.com/gravitational/trace"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/progress"
	"github.com/moby/buildkit/util/push"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// NewPush returns a new push action
func NewPush(group force.Group) func(Image) (force.Action, error) {
	return func(img Image) (force.Action, error) {
		pluginI, ok := group.GetVar(BuilderPlugin)
		if !ok {
			// plugin is not initialized, use defaults
			log.Debugf("Builder plugin is not initialized, using default")
			builder, err := New(Config{
				Context: group.Context(),
			})
			if err != nil {
				return nil, trace.Wrap(err)
			}
			return builder.NewPush(img)
		}
		return pluginI.(*Builder).NewPush(img)
	}
}

// NewPush returns a new push action that pushes
// the locally built container to the registry
func (b *Builder) NewPush(img Image) (force.Action, error) {
	if err := img.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	return &PushAction{
		Image:   img,
		Builder: b,
	}, nil
}

type PushAction struct {
	Builder *Builder
	Image   Image
}

func (b *PushAction) Run(ctx force.ExecutionContext) (force.ExecutionContext, error) {
	return ctx, b.Builder.Push(ctx.Context(), b.Image)
}

func (b *PushAction) String() string {
	return fmt.Sprintf("Push(tag=%v)", b.Image.Tag)
}

// Push pushes image to remote registry
func (b *Builder) Push(ctx context.Context, img Image) error {
	if err := img.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}

	log.Infof("Pushing %v.", img.String())

	sess, sessDialer, err := b.Session(ctx, img)
	if err != nil {
		return trace.Wrap(err, "failed to create session")
	}

	ctx = session.NewContext(ctx, sess.ID())
	ctx = namespaces.WithNamespace(ctx, "buildkit")
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return sess.Run(ctx, sessDialer)
	})
	eg.Go(func() error {
		defer sess.Close()
		return b.push(ctx, img.Tag, b.Config.Insecure)
	})

	if err := eg.Wait(); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Successfully pushed %v.", img.Tag)

	return nil
}

// push sends an image to a remote registry.
func (b *Builder) push(ctx context.Context, image string, insecure bool) error {
	// Parse the image name and tag.
	named, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return trace.BadParameter("parsing image name %q failed: %v", image, err)
	}
	// Add the latest lag if they did not provide one.
	named = reference.TagNameOnly(named)
	image = named.String()

	imgObj, err := b.opt.ImageStore.Get(ctx, image)
	if err != nil {
		return trace.BadParameter("getting image %q failed: %v", image, err)
	}
	ctxWithProgress := progress.WithProgress(ctx, &progressWriter{})
	return push.Push(ctxWithProgress, b.sessManager, b.opt.ContentStore,
		imgObj.Target.Digest, image, insecure, b.opt.ResolveOptionsFunc, false)
}

type progressWriter struct {
}

func (p *progressWriter) Write(id string, value interface{}) error {
	log.Infof("Progress -> %v %v.", id, value)
	return nil
}

func (p *progressWriter) Close() error {
	return nil
}
