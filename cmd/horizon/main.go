package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/verifa/horizon/pkg/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s, err := server.New(
		ctx,
		server.WithDevMode(),
		// server.WithGatewayOptions(
		// 	gateway.WithOIDCConfig(
		// 		gateway.OIDCConfig{
		// 			Issuer:       "http://localhost:9998/",
		// 			ClientID:     "web",
		// 			ClientSecret: "secret",
		// 			RedirectURL:  "http://localhost:9999/auth/callback",
		// 		},
		// 	),
		// ),
	)
	if err != nil {
		slog.Error("failed to start horizon", "error", err)
		os.Exit(1)
	}
	defer s.Close()
	slog.Info("horizon server started", "services", s.Services())

	<-ctx.Done()
	// Stop listening for interrupts so that a second interrupt will force
	// shutdown.
	stop()
	slog.Info(
		"interrupt received, shutting down horizon server",
	)
}
