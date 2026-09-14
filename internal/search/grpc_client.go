// Package search is the monolith-side client for search-indexer's
// SearchInternalService, wrapped in a circuit breaker with a MySQL
// fallback — see service.go and fallback_mysql.go.
package search

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
	"github.com/thvnhtai/gearshare/internal/auth"
)

type GRPCClient struct {
	conn   *grpc.ClientConn
	client gearsharev1.SearchInternalServiceClient
}

// NewGRPCClient dials search-indexer's SearchInternalService. basicUser/
// basicPass must match the credential search-indexer's own gRPC server was
// started with (auth.BasicAuthInterceptor) — every call is attached via
// auth.BasicAuthClientInterceptor, since the server-side check exists
// specifically to reject calls that don't carry it.
func NewGRPCClient(addr, basicUser, basicPass string) (*GRPCClient, error) {
	// Internal service-to-service traffic within the docker-compose/k8s
	// network; TLS termination for external traffic happens at Nginx
	// (deployments/nginx/nginx.conf) — see docs/security/owasp-top10-mapping.md.
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.BasicAuthClientInterceptor(basicUser, basicPass)),
	)
	if err != nil {
		return nil, fmt.Errorf("search: dial %s: %w", addr, err)
	}
	return &GRPCClient{conn: conn, client: gearsharev1.NewSearchInternalServiceClient(conn)}, nil
}

// maxSearchLimit bounds the page size sent over the wire — both a sane API
// guardrail and what makes `int32(limit)` below a checked, not merely
// assumed-safe, narrowing conversion.
const maxSearchLimit = 1000

func (c *GRPCClient) Search(ctx context.Context, query string, limit int) (*gearsharev1.SearchResponse, error) {
	if limit <= 0 {
		limit = 20
	} else if limit > maxSearchLimit {
		limit = maxSearchLimit
	}
	return c.client.Search(ctx, &gearsharev1.SearchRequest{Query: query, Limit: int32(limit)})
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
