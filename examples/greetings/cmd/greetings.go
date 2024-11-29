package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/examples/greetings"
	"github.com/verifa/horizon/pkg/controller"
	"github.com/verifa/horizon/pkg/hz"
)

func main() {
	if err := run(); err != nil {
		slog.Error("running", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Establish a connection to the NATS server.
	conn, err := nats.Connect(
		nats.DefaultURL,
		nats.UserCredentials("nats.creds"),
	)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	ctx := context.Background()
	// Handle interrupts.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	actor, err := hz.StartActor(
		ctx,
		conn,
		hz.WithActorActioner(
			greetings.GreetingsHelloAction{},
		),
	)
	if err != nil {
		return fmt.Errorf("start actor: %w", err)
	}
	defer func() {
		_ = actor.Stop()
	}()

	validator := greetings.GreetingValidator{}
	reconciler := greetings.GreetingReconciler{
		GreetingClient: hz.ObjectClient[greetings.Greeting]{
			Client: hz.NewClient(
				conn,
				hz.WithClientInternal(true),
				hz.WithClientManager("ctlr-greetings"),
			),
		},
	}
	ctlr, err := controller.StartController(
		ctx,
		conn,
		controller.WithControllerFor(greetings.Greeting{}),
		controller.WithControllerReconciler(&reconciler),
		controller.WithControllerValidator(&validator),
	)
	if err != nil {
		return fmt.Errorf("start controller: %w", err)
	}
	defer func() {
		_ = ctlr.Stop()
	}()

	portalHandler := greetings.PortalHandler{
		Conn: conn,
	}
	router := portalHandler.Router()
	portal, err := hz.StartPortal(ctx, conn, greetings.Portal, router)
	if err != nil {
		return fmt.Errorf("start portal: %w", err)
	}
	defer func() {
		_ = portal.Stop()
	}()

	<-ctx.Done()
	return nil
}
