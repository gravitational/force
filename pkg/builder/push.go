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
	"golang.org/x/sync/errgroup"
)

// NewPush specifies new push actions
type NewPush struct {
}

// NewInstance returns functions creating new push action
func (n *NewPush) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func(img interface{}) (force.Action, error) {
		return &PushAction{
			image: img,
		}, nil
	}
}

// PushAction returns new push actions
type PushAction struct {
	image interface{}
}

func (p *PushAction) Type() interface{} {
	return ""
}

// MarshalCode marshals the action into code representation
func (p *PushAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyPush,
		Args:    []interface{}{p.image},
	}
	return call.MarshalCode(ctx)
}

// Eval pushes image to remote repository
func (p *PushAction) Eval(ctx force.ExecutionContext) (interface{}, error) {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return nil, trace.NotFound("initialize Builder plugin in the setup section")
	}
	return pluginI.(*Builder).Push(ctx, p.image)
}

func (b *PushAction) String() string {
	return fmt.Sprintf("Push(image=%v)", b.image)
}

// Push pushes image to remote registry
func (b *Builder) Push(ectx force.ExecutionContext, iface interface{}) (interface{}, error) {
	var img Image
	if err := force.EvalInto(ectx, iface, &img); err != nil {
		return nil, trace.Wrap(err)
	}

	if err := img.CheckAndSetDefaults(); err != nil {
		return nil, trace.Wrap(err)
	}
	log := force.Log(ectx)

	log.Infof("Pushing image %v.", img.Tag)

	sess, sessDialer, err := b.Session(ectx, img)
	if err != nil {
		return nil, trace.Wrap(err, "failed to create session")
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
		return nil, trace.Wrap(err)
	}
	log.Infof("Successfully pushed %v.", img.Tag)

	return img.Tag, nil
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
