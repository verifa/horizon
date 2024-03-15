package greetings

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Validator) = (*GreetingValidator)(nil)

type GreetingValidator struct {
	hz.ZeroValidator
}

func (*GreetingValidator) ValidateCreate(
	ctx context.Context,
	data []byte,
) error {
	var greeting Greeting
	if err := json.Unmarshal(data, &greeting); err != nil {
		return fmt.Errorf("unmarshalling greeting: %w", err)
	}
	if greeting.Spec == nil {
		return fmt.Errorf("spec is required")
	}
	if greeting.Spec.Name == "" {
		return fmt.Errorf("name is required")
	}

	if !isFriend(greeting.Spec.Name) {
		return fmt.Errorf(
			"we don't greet strangers in Finland, we only know: %v",
			friends,
		)
	}
	return nil
}

var friends = []string{
	"Pekka", "Matti", "Jukka", "Kari", "Jari", "Mikko", "Ilkka",
}

func isFriend(name string) bool {
	return slices.Contains(friends, name)
}
