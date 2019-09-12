package builder

import (
	"github.com/gravitational/force"

	"github.com/gravitational/trace"
	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/worker/base"
	"golang.org/x/sync/errgroup"
)

// NewPrune specifies prune action - cleaning up
// dangled leftovers from builds - images and tags, layers
type NewPrune struct {
}

// NewInstance returns function creating new prune actions
func (n *NewPrune) NewInstance(group force.Group) (force.Group, interface{}) {
	return group, func() (force.Action, error) {
		return &PruneAction{}, nil
	}
}

// PruneAction prunes all leftover
// build layers and snapshots from the local storage
type PruneAction struct {
	builder *Builder
}

// Run runs prune action
func (p *PruneAction) Run(ctx force.ExecutionContext) error {
	pluginI, ok := ctx.Process().Group().GetPlugin(Key)
	if !ok {
		return trace.NotFound("initialize Builder plugin in the setup section")
	}
	return pluginI.(*Builder).Prune(ctx)
}

func (p *PruneAction) String() string {
	return "Prune()"
}

// MarshalCode marshals the action into code representation
func (p *PruneAction) MarshalCode(ctx force.ExecutionContext) ([]byte, error) {
	call := &force.FnCall{
		Package: string(Key),
		FnName:  KeyPrune,
	}
	return call.MarshalCode(ctx)
}

// Prune clears build cache
func (b *Builder) Prune(ectx force.ExecutionContext) error {
	log := force.Log(ectx)
	log.Infof("Prune.")

	ch := make(chan client.UsageInfo)

	// Create the new worker.
	w, err := base.NewWorker(*b.opt)
	if err != nil {
		return trace.Wrap(err)
	}

	eg, ctx := errgroup.WithContext(ectx)
	eg.Go(func() error {
		// Call prune on the worker.
		return w.Prune(ctx, ch)
	})

	eg2, ctx := errgroup.WithContext(ctx)
	eg2.Go(func() error {
		defer close(ch)
		return eg.Wait()
	})

	usage := []*controlapi.UsageRecord{}
	eg2.Go(func() error {
		for r := range ch {
			usage = append(usage, &controlapi.UsageRecord{
				ID:          r.ID,
				Mutable:     r.Mutable,
				InUse:       r.InUse,
				Size_:       r.Size,
				Parent:      r.Parent,
				UsageCount:  int64(r.UsageCount),
				Description: r.Description,
				CreatedAt:   r.CreatedAt,
				LastUsedAt:  r.LastUsedAt,
			})
		}

		return nil
	})

	return trace.Wrap(eg2.Wait())
}
