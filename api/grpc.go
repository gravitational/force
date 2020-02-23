package api

import (
	"net/http"
	"strings"

	"github.com/gravitational/force/proto"

	"github.com/gravitational/trace"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

// NewGRPCHandler returns a new instance of GRPC handler
func NewGRPCHandler(handler http.Handler) http.Handler {
	grpcServer := grpc.NewServer()
	proto.RegisterTickServiceServer(grpcServer, &Handler{})
	wrappedGrpcServer := grpcweb.WrapServer(grpcServer)

	return &GRPCHandler{
		log: log.WithFields(log.Fields{
			trace.Component: "grpc",
		}),
		httpHandler:    handler,
		webGRPCHandler: wrappedGrpcServer,
		grpcHandler:    grpcServer,
	}
}

// GRPCHandler is GPRC handler middleware
type GRPCHandler struct {
	log *log.Entry
	// httpHandler is a server serving HTTP API
	httpHandler http.Handler
	// webGRPCHandler is golang GRPC handler
	// that uses web sockets as a transport and
	// is used by the UI
	webGRPCHandler *grpcweb.WrappedGrpcServer
	// grpcHandler is a GPRC standard handler
	grpcHandler *grpc.Server
}

// ServeHTTP dispatches requests based on the request type
func (g *GRPCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.webGRPCHandler.IsGrpcWebRequest(r) {
		g.webGRPCHandler.ServeHTTP(w, r)
	} else if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
		// magic combo match signifying GRPC request
		// https://grpc.io/blog/coreos
		g.grpcHandler.ServeHTTP(w, r)
	} else {
		g.httpHandler.ServeHTTP(w, r)
	}
}
