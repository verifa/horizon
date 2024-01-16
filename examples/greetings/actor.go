package greetings

import (
	"context"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Action[Greeting]) = (*GreetingsHelloAction)(nil)

type GreetingsHelloAction struct{}

// Action implements hz.Action.
func (*GreetingsHelloAction) Action() string {
	return "hello"
}

// Do implements hz.Action.
func (*GreetingsHelloAction) Do(
	ctx context.Context,
	greeting Greeting,
) (Greeting, error) {
	fmt.Println("Greetings, " + *greeting.Spec.Name + "!")
	greeting.Status.Ready = true
	greeting.Status.Phase = StatusPhaseCompleted
	return greeting, nil
}
