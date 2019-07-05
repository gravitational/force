package builder

import (
	"context"
	"path/filepath"
	"time"

	controlapi "github.com/moby/buildkit/api/services/control"
	"github.com/moby/buildkit/control"
	"github.com/moby/buildkit/frontend"
	dockerbuilder "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/frontend/gateway"
	"github.com/moby/buildkit/frontend/gateway/forwarder"
	"github.com/moby/buildkit/solver/bboltcachestorage"
	"github.com/moby/buildkit/worker"
	"github.com/moby/buildkit/worker/base"

	"github.com/gravitational/trace"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// solve calls Solve on the controller.
func (b *Builder) solve(ctx context.Context, req *controlapi.SolveRequest, ch chan *controlapi.StatusResponse) error {
	defer close(ch)

	statusCtx, cancelStatus := context.WithCancel(ctx)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		defer func() { // make sure the Status ends cleanly on build errors
			go func() {
				<-time.After(3 * time.Second)
				cancelStatus()
			}()
		}()
		_, err := b.controller.Solve(ctx, req)
		if err != nil {
			return trace.Wrap(err, "failed to solve")
		}
		return nil
	})

	eg.Go(func() error {
		srv := &controlStatusServer{
			ctx: statusCtx,
			ch:  ch,
		}
		return b.controller.Status(&controlapi.StatusRequest{
			Ref: req.Ref,
		}, srv)
	})
	return eg.Wait()
}

func (b *Builder) createController() (*control.Controller, error) {
	// Create the worker opts.
	opt, err := b.createWorkerOpt(b.Context, true)
	if err != nil {
		return nil, trace.Wrap(err, "creating worker opt failed")
	}

	// Create the new worker.
	w, err := base.NewWorker(opt)
	if err != nil {
		return nil, trace.Wrap(err, "creating worker failed")
	}

	// Create the worker controller.
	wc := &worker.Controller{}
	if err := wc.Add(w); err != nil {
		return nil, trace.Wrap(err, "adding worker to worker controller failed")
	}

	// Add the frontends.
	frontends := map[string]frontend.Frontend{}
	frontends["dockerfile.v0"] = forwarder.NewGatewayForwarder(wc, dockerbuilder.Build)
	frontends["gateway.v0"] = gateway.NewGatewayFrontend(wc)

	// Create the cache storage
	cacheStorage, err := bboltcachestorage.NewStore(filepath.Join(b.root, "cache.db"))
	if err != nil {
		return nil, trace.Wrap(err)
	}

	// Create the controller.
	controller, err := control.NewController(control.Opt{
		SessionManager:   b.sessManager,
		WorkerController: wc,
		Frontends:        frontends,
		CacheKeyStorage:  cacheStorage,
		// No cache importer/exporter
	})
	if err != nil {
		return nil, trace.Wrap(err, "creating new controller failed")
	}
	return controller, nil
}

type controlStatusServer struct {
	ctx               context.Context
	ch                chan *controlapi.StatusResponse
	grpc.ServerStream // dummy
}

func (x *controlStatusServer) SendMsg(m interface{}) error {
	return x.Send(m.(*controlapi.StatusResponse))
}

func (x *controlStatusServer) Send(m *controlapi.StatusResponse) error {
	x.ch <- m
	return nil
}

func (x *controlStatusServer) Context() context.Context {
	return x.ctx
}
