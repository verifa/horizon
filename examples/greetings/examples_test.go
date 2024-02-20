package greetings_test

import (
	"context"
	"log/slog"
	"os"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/examples/greetings"
	"github.com/verifa/horizon/pkg/hz"
)

func ExampleGreeting() {
	ctx := context.Background()
	nc, err := nats.Connect(nats.DefaultURL, nats.UserCredentials("nats.creds"))
	if err != nil {
		slog.Error("connecting to nats", "error", err)
		os.Exit(1)
	}
	client := hz.InternalClient(nc)
	greetClient := hz.ObjectClient[greetings.Greeting]{Client: client}

	validr := greetings.GreetingValidator{}
	recon := greetings.GreetingReconciler{
		GreetingClient: greetClient,
	}
	ctlr, err := hz.StartController(
		ctx,
		nc,
		hz.WithControllerFor(greetings.Greeting{}),
		hz.WithControllerValidator(&validr),
		hz.WithControllerReconciler(&recon),
	)
	if err != nil {
		slog.Error("starting greeting controller", "error", err)
		os.Exit(1)
	}
	defer ctlr.Stop()

	action := greetings.GreetingsHelloAction{}
	actor, err := hz.StartActor(
		ctx,
		nc,
		hz.WithActorActioner(&action),
	)
	if err != nil {
		slog.Error("starting greeting actor", "error", err)
		os.Exit(1)
	}
	defer actor.Stop()
}
