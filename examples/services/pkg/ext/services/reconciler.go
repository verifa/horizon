package services

import (
	"context"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
)

type Reconciler struct {
	Client hz.ObjectClient[Service]
}

// Reconcile implements hz.Reconciler.
func (r *Reconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	service, err := r.Client.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}
	applyService, err := hz.ExtractManagedFields(
		service,
		r.Client.Client.Manager,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}
	if service.DeletionTimestamp.IsPast() {
		// Handle any cleanup logic here.
		return hz.Result{}, nil
	}
	// TODO: Implement the reconcile logic here.
	// For example, call Terraform, deploy some Kubernetes stuff or use Go SDKs
	// to create cloud resources.
	//
	// For now, just set the status as ready.
	applyService.Status = &ServiceStatus{
		Ready: true,
	}
	if _, err := r.Client.Apply(ctx, applyService); err != nil {
		return hz.Result{}, fmt.Errorf("updating counter: %w", err)
	}

	return hz.Result{}, nil
}
