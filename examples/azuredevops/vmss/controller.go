package vmss

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/verifa/horizon/examples/azuredevops/terra"
	"github.com/verifa/horizon/examples/azuredevops/vmss/tf"
	"github.com/verifa/horizon/pkg/hz"
)

//go:embed tf/*.tf
var terraformFS embed.FS

const (
	ManagerName = "ctlr-vmss"
	Finalizer   = "azuredevops/vmss"
)

var _ hz.Reconciler = (*Reconciler)(nil)

type Reconciler struct {
	SubscriptionID string
	SubnetID       string
	Client         hz.Client
}

// Reconcile implements hz.Reconciler.
func (r *Reconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	vmssClient := hz.ObjectClient[VMScaleSet]{
		Client: r.Client,
	}
	vmScaleSet, err := vmssClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}

	applyVMSS, err := hz.ExtractManagedFields(vmScaleSet, ManagerName)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}

	tfVars := tf.Vars{
		VMScaleSet: &tf.VMScaleSet{
			Name:              vmScaleSet.Name,
			ResourceGroupName: vmScaleSet.Spec.ResourceGroupName,
			Location:          vmScaleSet.Spec.Location,
			Sku:               vmScaleSet.Spec.VMSize,
			SubnetID:          r.SubnetID,
		},
	}

	reconcileOptions := []terra.ReconcileOption{
		terra.WithFS(terraformFS),
		terra.WithFSSub("tf"),
		terra.WithBackend(terra.BackendAzureRM{
			ResourceGroupName:  "rg-default",
			StorageAccountName: "verifahorizon",
			ContainerName:      "tfstate",
			Key:                hz.KeyFromObject(vmScaleSet),
		}),
		terra.WithTFVars(tfVars),
		terra.WithWorkdir(
			filepath.Join(
				os.TempDir(),
				"terraform",
				hz.KeyFromObject(vmScaleSet),
			),
		),
	}

	if vmScaleSet.DeletionTimestamp.IsPast() {
		slog.Info(
			"azure vmss deletion requested",
			"name",
			vmScaleSet.Name,
			"status",
			vmScaleSet.Status,
			"finalizers",
			vmScaleSet.Finalizers,
		)
		// If finalizer not set, no need to delete.
		if !vmScaleSet.Finalizers.Contains(Finalizer) {
			slog.Info("finalizer not set, skipping")
			return hz.Result{}, nil
		}
		reconcileOptions = append(
			reconcileOptions,
			terra.WithDestroy(true),
		)
		_, err = terra.Reconcile(
			ctx,
			reconcileOptions...,
		)
		if err != nil {
			return hz.Result{}, fmt.Errorf("reconciling terraform: %w", err)
		}
		applyVMSS.Status = nil
		// Remove finzliers.
		applyVMSS.Finalizers = &hz.Finalizers{}
		if err := vmssClient.Apply(ctx, applyVMSS, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying vmss: %w", err)
		}
		return hz.Result{}, nil
	}

	tfVMSSOutput := tf.OutputVMScaleSet{}
	reconcileOptions = append(
		reconcileOptions,
		terra.WithOutputs(&tfVMSSOutput),
	)
	_, err = terra.Reconcile(
		ctx,
		reconcileOptions...,
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("reconciling terraform: %w", err)
	}
	status := vmScaleSet.Status
	newStatus := VMScaleSetStatus{
		Ready: true,
		ID:    tfVMSSOutput.ID,
	}
	if status == nil || *status != newStatus {
		applyVMSS.Status = &newStatus
		if err := vmssClient.Apply(ctx, applyVMSS, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying vmss: %w", err)
		}
	}

	return hz.Result{}, nil
}
