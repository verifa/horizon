package greetings_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verifa/horizon/examples/greetings"
	"github.com/verifa/horizon/pkg/hz"
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
			Name: hz.P("Pekka"),
		},
	}
	err = greetClient.Apply(ctx, greeting)
	if err != nil {
		t.Fatal("applying greeting: ", err)
	}

	//
	// Verify that the controller reconciles the object.
	//
	// Create a timeout and a done channel.
	// Watch until the greeting is ready.
	// If the timeout is reached, fail the test.
	//
	timeout := time.After(time.Second * 5)
	done := make(chan struct{})
	watcher, err := hz.StartWatcher(
		ctx,
		ts.Conn,
		hz.WithWatcherFor(greeting),
		hz.WithWatcherFn(
			func(event hz.Event) (hz.Result, error) {
				var watchGreeting greetings.Greeting
				if err := json.Unmarshal(event.Data, &watchGreeting); err != nil {
					return hz.Result{}, fmt.Errorf(
						"unmarshalling greeting: %w",
						err,
					)
				}
				if watchGreeting.Status == nil {
					return hz.Result{}, nil
				}
				if watchGreeting.Status.Ready == true {
					close(done)
				}
				return hz.Result{}, nil
			},
		),
	)
	if err != nil {
		t.Fatal("starting greeting watcher: ", err)
	}
	defer watcher.Close()

	select {
	case <-timeout:
		t.Fatal("timed out waiting for account")
	case <-done:
	}
}
