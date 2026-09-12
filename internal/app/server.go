package app

import (
	"context"
	"fmt"
	"net/http"

	"github.com/thvnhtai/gearshare/internal/config"
)

type Server struct {
	httpServer *http.Server
	cfg        config.HTTPConfig
}

func NewServer(handler http.Handler, cfg config.HTTPConfig) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         cfg.Addr,
			Handler:      handler,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
		cfg: cfg,
	}
}

func (s *Server) Start() error {
	fmt.Printf("gearshare-api: HTTP listening on %s\n", s.cfg.Addr)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
