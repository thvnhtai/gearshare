package auth

import (
	"context"
	"encoding/base64"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// BasicAuthInterceptor is the gRPC-side equivalent of RequireBasicAuth —
// used by every service's internal gRPC server (cmd/api, cmd/notification-service,
// cmd/search-indexer) to guard reflection/health/service-to-service calls with
// the same simple, operator-shared credential rather than end-user JWTs.
func BasicAuthInterceptor(username, password string) grpc.UnaryServerInterceptor {
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		values := md.Get("authorization")
		if len(values) == 0 || strings.TrimSpace(values[0]) != expected {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		return handler(ctx, req)
	}
}
