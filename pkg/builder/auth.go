package builder

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/moby/buildkit/session/auth"
	"google.golang.org/grpc"
)

func (b *Builder) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, b)
}

func (b *Builder) Credentials(ctx context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	logger := b.cfg.group.Logger()
	logger.Debugf("Credentials request %v.", req)

	// default registry - no login supported
	if req.Host == "registry-1.docker.io" {
		return &auth.CredentialsResponse{}, nil
	}

	if b.cfg.server == "" {
		return nil, trace.NotFound("no credentials use BuilderConfig{Username: `...`, Secret: `...`, Server: %q} to setup credentials", req.Host)
	}

	if b.cfg.server != req.Host {
		return nil, trace.NotFound("no credentials found for %q, only for %q", req.Host, b.cfg.server)
	}

	logger.Debugf("Authorized as %v in %v.", b.cfg.username, req.Host)
	return &auth.CredentialsResponse{
		Username: string(b.cfg.username),
		Secret:   string(b.cfg.secret),
	}, nil
}
