package greetings

import (
	"context"
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
	greeting, err := r.GreetingClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}
	applyGreet, err := hz.ExtractManagedFields(
		greeting,
		r.GreetingClient.Client.Manager,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}
	if greeting.DeletionTimestamp.IsPast() {
		// Handle any cleanup logic here.
		return hz.Result{}, nil
	}

	// Obviously we don't need to run an action here, but this is just an
	// example.
	reply, err := r.GreetingClient.Run(
		ctx,
		&GreetingsHelloAction{},
		greeting,
	)
	if err != nil {
		applyGreet.Status = &GreetingStatus{
			Ready:    false,
			Error:    fmt.Sprintf("running hello action: %s", err),
			Response: "",
		}
		if _, err := r.GreetingClient.Apply(ctx, applyGreet); err != nil {
			return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
		}
		return hz.Result{}, fmt.Errorf("running hello action: %w", err)
	}

	applyGreet.Status = &GreetingStatus{
		Ready:    true,
		Error:    "",
		Response: reply.Status.Response,
	}
	if _, err := r.GreetingClient.Apply(ctx, applyGreet); err != nil {
		return hz.Result{}, fmt.Errorf("updating greeting: %w", err)
	}

	return hz.Result{}, nil
}
