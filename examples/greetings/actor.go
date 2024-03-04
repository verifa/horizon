package greetings

import (
	"context"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Action[Greeting]) = (*GreetingsHelloAction)(nil)

type GreetingsHelloAction struct{}

// Action implements hz.Action.
func (GreetingsHelloAction) Action() string {
	return "hello"
}

// Do implements hz.Action.
func (a GreetingsHelloAction) Do(
	ctx context.Context,
	greeting Greeting,
) (Greeting, error) {
	if err := a.validate(greeting); err != nil {
		return greeting, fmt.Errorf("validating greeting: %w", err)
	}
	greeting.Status = &GreetingStatus{
		Ready:    true,
		Phase:    StatusPhaseCompleted,
		Response: "Greetings, " + *greeting.Spec.Name + "!",
	}
	return greeting, nil
}

func (a GreetingsHelloAction) validate(greeting Greeting) error {
	if greeting.Spec == nil {
		return fmt.Errorf("spec is required")
	}
	if greeting.Spec.Name == nil {
		return fmt.Errorf("name is required")
	}

	if !isFriend(*greeting.Spec.Name) {
		return fmt.Errorf(
			"we don't greet strangers in Finland, we only know: %v",
			friends,
		)
	}
	return nil
}
