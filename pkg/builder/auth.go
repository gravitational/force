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
	logger := b.Group.Logger()
	logger.Debugf("Credentials request %v.", req)

	// default registry - no login supported
	if req.Host == "registry-1.docker.io" {
		return &auth.CredentialsResponse{}, nil
	}

	if b.Config.Server == "" {
		return nil, trace.NotFound("no credentials use BuilderConfig{Username: `...`, Secret: `...`, Server: %q} to setup credentials", req.Host)
	}

	if b.Config.Server != req.Host {
		return nil, trace.NotFound("no credentials found for %q, only for %q", req.Host, b.Config.Server)
	}

	logger.Debugf("Authorized as %v in %v.", b.Config.Username, req.Host)
	return &auth.CredentialsResponse{
		Username: b.Config.Username,
		Secret:   b.Config.Secret,
	}, nil
}
