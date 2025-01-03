package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"

	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/server"
)

func main() {
	if err := run(); err != nil {
		slog.Error("horizon server failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s, err := server.Start(
		ctx,
		server.WithDevMode(),
		server.WithAuthOptions(auth.WithAdminGroups("admin")),
	)
	if err != nil {
		return err
	}
	defer s.Close()
	slog.Info("horizon server started")

	<-ctx.Done()
	// Stop listening for interrupts so that a second interrupt will force
	// shutdown.
	stop()
	slog.Info(
		"interrupt received, shutting down horizon server",
	)
	return nil
}
