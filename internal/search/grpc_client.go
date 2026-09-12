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
)

type GRPCClient struct {
	conn   *grpc.ClientConn
	client gearsharev1.SearchInternalServiceClient
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	// Internal service-to-service traffic within the docker-compose/k8s
	// network; TLS termination for external traffic happens at Nginx
	// (deployments/nginx/nginx.conf) — see docs/security/owasp-top10-mapping.md.
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("search: dial %s: %w", addr, err)
	}
	return &GRPCClient{conn: conn, client: gearsharev1.NewSearchInternalServiceClient(conn)}, nil
}

func (c *GRPCClient) Search(ctx context.Context, query string, limit int) (*gearsharev1.SearchResponse, error) {
	return c.client.Search(ctx, &gearsharev1.SearchRequest{Query: query, Limit: int32(limit)})
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
