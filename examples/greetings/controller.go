package greetings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

type GreetingReconciler struct {
	GreetingClient hz.ObjectClient[Greeting]
}

// Reconcile implements hz.Reconciler.
func (r *GreetingReconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	greeting, err := r.GreetingClient.Get(ctx, req.Key)
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}
	// Obviously we don't need to run an action here, but this is just an
	// example.
	reply, err := r.GreetingClient.Run(
		ctx,
		&GreetingsHelloAction{},
		*greeting,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("running hello action: %w", err)
	}

	if err := r.GreetingClient.Apply(ctx, reply, hz.WithApplyManager("greeting-ctlr")); err != nil {
		return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
	}

	return hz.Result{}, nil
}

var _ (hz.Validator) = (*GreetingValidator)(nil)

type GreetingValidator struct{}

// Validate implements hz.Validator.
func (*GreetingValidator) Validate(ctx context.Context, data []byte) error {
	var greeting Greeting
	if err := json.Unmarshal(data, &greeting); err != nil {
		return fmt.Errorf("unmarshalling greeting: %w", err)
	}
	if greeting.Spec.Name == nil {
		return fmt.Errorf("name is required")
	}

	if !isFriend(*greeting.Spec.Name) {
		return fmt.Errorf("we don't greet strangers in Finland, terribly sorry")
	}
	return nil
}

func isFriend(name string) bool {
	return name == "Alice" || name == "Bob"
}
