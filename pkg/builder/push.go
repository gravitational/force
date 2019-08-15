package builder

import (
	"context"
	"fmt"

	"github.com/gravitational/force"

	"github.com/containerd/containerd/namespaces"
	"github.com/docker/distribution/reference"
	"github.com/gravitational/trace"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/push"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// NewPush specifies new push actions
type NewPush struct {
}

// NewInstance returns functions creating new push action
func (n *NewPush) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(img Image) (force.Action, error) {
		pluginI, ok := group.GetPlugin(Plugin)
		if !ok {
			// plugin is not initialized, use defaults
			logrus.Debugf("Builder plugin is not initialized, using default")
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
	return &PushAction{
		Image:   img,
		Builder: b,
	}, nil
}

// PushAction returns new push actions
type PushAction struct {
	Builder *Builder
	Image   Image
}

func (b *PushAction) Run(ctx force.ExecutionContext) error {
	return b.Builder.Push(ctx, b.Image)
}

func (b *PushAction) String() string {
	return fmt.Sprintf("Push(tag=%v)", b.Image.Tag)
}

// Push pushes image to remote registry
func (b *Builder) Push(ectx force.ExecutionContext, img Image) error {
	if err := img.CheckAndSetDefaults(ectx); err != nil {
		return trace.Wrap(err)
	}

	log := force.Log(ectx)
	tag, err := img.Tag.Eval(ectx)
	if err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Pushing image %v.", tag)

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
		return b.push(ctx, tag, b.Config.Insecure)
	})

	if err := eg.Wait(); err != nil {
		return trace.Wrap(err)
	}
	log.Infof("Successfully pushed %v.", tag)

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
