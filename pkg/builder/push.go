package builder

import (
	"context"

	"github.com/gravitational/force"

	"github.com/containerd/containerd/namespaces"
	"github.com/docker/distribution/reference"
	"github.com/gravitational/trace"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/push"
	"golang.org/x/sync/errgroup"
)

func Push(ctx force.ExecutionContext, img Image) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize Builder plugin in the setup section")
	}
	return pluginI.(*Builder).Push(ctx, img)
}

// Push pushes image to remote registry
func (b *Builder) Push(ectx force.ExecutionContext, img Image) error {
	if err := img.CheckAndSetDefaults(); err != nil {
		return trace.Wrap(err)
	}
	log := force.Log(ectx)

	log.Infof("Pushing image %v.", img.Tag)

	sess, sessDialer, err := b.Session(ectx, img)
	if err != nil {
		return trace.Wrap(err, "failed to create session")
	}

	ctx := session.NewContext(ectx, sess.ID())
	ctx = namespaces.WithNamespace(ctx, "buildkit")
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return sess.Run(ctx, sessDialer)
	})
	eg.Go(func() error {
		defer sess.Close()
		return b.push(ctx, img.Tag, b.cfg.Insecure)
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
	return push.Push(ctx, b.sessManager, b.opt.ContentStore,
		imgObj.Target.Digest, image, insecure, b.opt.ResolveOptionsFunc, false)
}
