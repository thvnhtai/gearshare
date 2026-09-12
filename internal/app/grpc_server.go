package app

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	gearsharev1 "github.com/thvnhtai/gearshare/api/gen/gearshare/v1"
	"github.com/thvnhtai/gearshare/internal/auth"
	"github.com/thvnhtai/gearshare/internal/user"
)

// userInternalServer implements gearsharev1.UserInternalServiceServer,
// backing notification-service's contact-detail lookups so notification
// payloads never need to carry PII themselves (see docs/architecture.md).
type userInternalServer struct {
	gearsharev1.UnimplementedUserInternalServiceServer
	users *user.Repository
}

func (s *userInternalServer) GetUserContact(ctx context.Context, req *gearsharev1.GetUserContactRequest) (*gearsharev1.UserContact, error) {
	u, err := s.users.GetByID(ctx, req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return &gearsharev1.UserContact{
		UserId:      u.ID,
		DisplayName: u.DisplayName,
		Email:       u.Email,
	}, nil
}

// NewGRPCServer builds the monolith's internal gRPC server: the
// UserInternalService implementation, plus standard health/reflection
// services for operability, all behind the same Basic Auth style used for
// REST's /internal/* routes (internal/auth.BasicAuthInterceptor).
func NewGRPCServer(users *user.Repository, basicUser, basicPass string) *grpc.Server {
	srv := grpc.NewServer(grpc.UnaryInterceptor(auth.BasicAuthInterceptor(basicUser, basicPass)))

	gearsharev1.RegisterUserInternalServiceServer(srv, &userInternalServer{users: users})

	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, healthSrv)

	reflection.Register(srv)
	return srv
}

func ServeGRPC(srv *grpc.Server, addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return srv.Serve(lis)
}
