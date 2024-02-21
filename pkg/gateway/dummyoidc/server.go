package dummyoidc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"golang.org/x/exp/slog"

	"github.com/verifa/horizon/pkg/gateway/dummyoidc/exampleop"
	"github.com/verifa/horizon/pkg/gateway/dummyoidc/storage"
	"github.com/zitadel/oidc/v3/pkg/op"
)

func Start(ctx context.Context, config Config) (*Server, error) {
	s := Server{}
	if err := s.Start(ctx, config); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}
	return &s, nil
}

type Config struct {
	Port int

	Users map[string]*storage.User
}

type Server struct {
	Issuer string
	http   *http.Server
}

func (s *Server) Start(ctx context.Context, config Config) error {
	issuer := fmt.Sprintf("http://localhost:%d/", config.Port)
	s.Issuer = issuer
	// The OpenIDProvider interface needs a Storage interface handling various
	// checks and state manipulations
	// this might be the layer for accessing your database
	// in this example it will be handled in-memory.
	storage := storage.NewStorage(storage.NewUserStore(config.Users))

	logger := slog.New(
		slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}),
	)
	router := exampleop.SetupServer(
		issuer,
		storage,
		logger,
		false,
		op.WithCustomUserinfoEndpoint(&op.Endpoint{}),
	)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.Port),
		Handler: router,
	}
	s.http = server
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info(
		"server listening, press ctrl+c to stop",
		"addr",
		fmt.Sprintf("http://localhost:%d/", config.Port),
	)
	go func() {
		if err := server.Serve(l); err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http server", "error", err.Error())
			}
		}
	}()
	return nil
}

func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	if err := s.http.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
