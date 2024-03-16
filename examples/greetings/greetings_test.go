package greetings_test

import (
	"context"
	"testing"
	"time"

	"github.com/verifa/horizon/examples/greetings"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hztest"
	"github.com/verifa/horizon/pkg/server"
)

func TestGreeting(t *testing.T) {
	ctx := context.Background()
	// Create a test server which includes the core of Horizon.
	ts := server.Test(t, ctx)
	client := hz.NewClient(
		ts.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("ctlr-greeting"),
	)
	greetClient := hz.ObjectClient[greetings.Greeting]{Client: client}

	//
	// Setup greetings controller with validator and reconciler.
	//
	validr := greetings.GreetingValidator{}
	recon := greetings.GreetingReconciler{
		GreetingClient: greetClient,
	}
	ctlr, err := hz.StartController(
		ctx,
		ts.Conn,
		hz.WithControllerFor(greetings.Greeting{}),
		hz.WithControllerValidator(&validr),
		hz.WithControllerReconciler(&recon),
	)
	if err != nil {
		t.Fatal("starting greeting controller: ", err)
	}
	defer ctlr.Stop()

	//
	// Setup greetings actor.
	//
	action := greetings.GreetingsHelloAction{}
	actor, err := hz.StartActor(
		ctx,
		ts.Conn,
		hz.WithActorActioner(action),
	)
	if err != nil {
		t.Fatal("starting greeting actor: ", err)
	}
	defer actor.Stop()

	//
	// Apply a greetings object.
	//
	greeting := greetings.Greeting{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "Pekka",
		},
		Spec: &greetings.GreetingSpec{
			Name: "Pekka",
		},
	}
	_, err = greetClient.Apply(ctx, greeting)
	if err != nil {
		t.Fatal("applying greeting: ", err)
	}

	//
	// Verify that the controller reconciles the object.
	//
	// Watch until the greeting is ready.
	// If the timeout is reached, the test fails.
	//
	hztest.WatchWaitUntil(
		t,
		ctx,
		ts.Conn,
		time.Second*5,
		greeting,
		func(greeting greetings.Greeting) bool {
			if greeting.Status == nil {
				return false
			}
			if greeting.Status.Ready == true {
				return true
			}
			return false
		},
	)
}
