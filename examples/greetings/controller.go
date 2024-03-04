package greetings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
	"golang.org/x/exp/slices"
)

type GreetingReconciler struct {
	GreetingClient hz.ObjectClient[Greeting]
}

// Reconcile implements hz.Reconciler.
func (r *GreetingReconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	greeting, err := r.GreetingClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}
	if greeting.DeletionTimestamp.IsPast() {
		// Handle any cleanup logic here.
		return hz.Result{}, nil
	}
	applyGreet, err := hz.ExtractManagedFields(
		greeting,
		r.GreetingClient.Client.Manager,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}

	// Obviously we don't need to run an action here, but this is just an
	// example.
	reply, err := r.GreetingClient.Run(
		ctx,
		&GreetingsHelloAction{},
		greeting,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("running hello action: %w", err)
	}

	// If status is nil then set it.
	if greeting.Status == nil {
		applyGreet.Status = reply.Status

		if err := r.GreetingClient.Apply(ctx, applyGreet); err != nil {
			return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
		}
		// Return here, because the above apply will trigger a reconcile.
		return hz.Result{}, nil
	}

	// If the current response does not match, or the greeting is not ready,
	// update the status.
	if greeting.Status.Response != reply.Status.Response ||
		!greeting.Status.Ready {
		if !greeting.Status.Ready {
			applyGreet.Status = &GreetingStatus{
				Ready:         true,
				FailureReason: "",
				Phase:         StatusPhaseCompleted,
				Response:      reply.Status.Response,
			}
			if err := r.GreetingClient.Apply(ctx, applyGreet); err != nil {
				return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
			}
			return hz.Result{}, nil
		}
		return hz.Result{}, nil
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

var friends = []string{
	"Pekka", "Matti", "Jukka", "Kari", "Jari", "Mikko", "Ilkka",
}

func isFriend(name string) bool {
	return slices.Contains(friends, name)
}
