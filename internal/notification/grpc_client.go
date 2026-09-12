package notification

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
)

type GRPCClient struct {
	conn   *grpc.ClientConn
	client gearsharev1.NotificationServiceClient
}

func NewGRPCClient(addr string) (*GRPCClient, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("notification: dial %s: %w", addr, err)
	}
	return &GRPCClient{conn: conn, client: gearsharev1.NewNotificationServiceClient(conn)}, nil
}

func (c *GRPCClient) SendTransactional(ctx context.Context, userID int64, template string, data map[string]string) error {
	_, err := c.client.SendTransactional(ctx, &gearsharev1.SendTransactionalRequest{
		UserId:   userID,
		Template: template,
		Data:     data,
	})
	return err
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}
