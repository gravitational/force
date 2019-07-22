package builder

import (
	"context"

	"github.com/gravitational/trace"
	"github.com/moby/buildkit/session/auth"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

func (b *Builder) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, b)
}

func (b *Builder) Credentials(ctx context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	log.Debugf("Credentials request %v.", req)

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

	log.Debugf("Authorized as %v in %v -> %#v", b.Config.Username, req.Host, b.Config)
	return &auth.CredentialsResponse{
		Username: b.Config.Username,
		Secret:   b.Config.Secret,
	}, nil
}
