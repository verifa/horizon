package agentpool

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/verifa/horizon/examples/azuredevops/agentpool/tf"
	"github.com/verifa/horizon/examples/azuredevops/project"
	"github.com/verifa/horizon/examples/azuredevops/terra"
	"github.com/verifa/horizon/examples/azuredevops/vmss"
	"github.com/verifa/horizon/pkg/hz"
	"golang.org/x/exp/slog"
)

//go:embed tf/*.tf
var terraformFS embed.FS

const (
	ManagerName = "ctlr-agentpool"
	Finalizer   = "azuredevops/agentpool"
)

var _ hz.Reconciler = (*Reconciler)(nil)

type Reconciler struct {
	Client hz.Client
}

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	agentPoolClient := hz.ObjectClient[AgentPool]{Client: r.Client}
	vmssClient := hz.ObjectClient[vmss.VMScaleSet]{Client: r.Client}
	projectClient := hz.ObjectClient[project.Project]{Client: r.Client}

	agentPool, err := agentPoolClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}

	applyAgentPool, err := hz.ExtractManagedFields(agentPool, ManagerName)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}

	reconcileOptions := []terra.ReconcileOption{
		terra.WithFS(terraformFS),
		terra.WithFSSub("tf"),
		terra.WithBackend(terra.BackendAzureRM{
			ResourceGroupName:  "rg-default",
			StorageAccountName: "verifahorizon",
			ContainerName:      "tfstate",
			Key:                hz.KeyFromObject(agentPool),
		}),
		terra.WithWorkdir(
			filepath.Join(
				os.TempDir(),
				"terraform",
				hz.KeyFromObject(agentPool),
			),
		),
	}

	// Handle deletion.
	if agentPool.ObjectMeta.DeletionTimestamp.IsPast() {
		slog.Info(
			"delete scheduled for",
			"name",
			agentPool.Name,
			"status",
			agentPool.Status,
			"finalizers",
			agentPool.Finalizers,
		)
		if !agentPool.Finalizers.Contains(Finalizer) {
			return hz.Result{}, nil
		}
		if agentPool.Status != nil {
			slog.Info("deleting agent pool", "id", agentPool.Name)
			reconcileOptions = append(
				reconcileOptions,
				terra.WithDestroy(true),
				terra.WithTFVars(agentPool.Status.TFVars),
			)
			_, err = terra.Reconcile(
				ctx,
				reconcileOptions...,
			)
			if err != nil {
				return hz.Result{}, fmt.Errorf("reconciling terraform: %w", err)
			}
		}
		applyAgentPool.Finalizers = &hz.Finalizers{}
		applyAgentPool.Status = nil
		if err := agentPoolClient.Apply(ctx, applyAgentPool, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying agent pool: %w", err)
		}
		return hz.Result{}, nil
	}

	projectRef, err := projectClient.Get(
		ctx,
		hz.WithGetKey(hz.ObjectKey{
			Account: agentPool.Account,
			Name:    agentPool.Spec.ProjectRef.Name,
		}),
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("getting project ref: %w", err)
	}
	if projectRef.Status == nil || !projectRef.Status.Ready {
		return hz.Result{}, fmt.Errorf("project ref is not ready")
	}

	vmScaleSetRef, err := vmssClient.Get(
		ctx,
		hz.WithGetKey(hz.ObjectKey{
			Account: agentPool.Account,
			Name:    agentPool.Spec.VMScaleSetRef.Name,
		}),
	)
	if err != nil {
		return hz.Result{}, fmt.Errorf("getting VMScaleSet ref: %w", err)
	}

	if vmScaleSetRef.Status == nil || !vmScaleSetRef.Status.Ready {
		return hz.Result{}, fmt.Errorf("VMScaleSet ref is not ready")
	}

	tfVars := tf.Vars{
		AgentPool: &tf.AgentPool{
			Name:                agentPool.Name,
			ProjectID:           projectRef.Status.ID,
			ServiceConnectionID: projectRef.Status.ServiceConnectionID,
			VMScaleSetID:        vmScaleSetRef.Status.ID,
		},
	}

	reconcileOptions = append(reconcileOptions, terra.WithTFVars(tfVars))
	_, err = terra.Reconcile(
		ctx,
		reconcileOptions...,
	)
	if err != nil {
		applyAgentPool.Status = &AgentPoolStatus{
			Ready:  false,
			TFVars: tfVars,
		}
		if err := agentPoolClient.Apply(ctx, applyAgentPool, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying agent pool: %w", err)
		}
		return hz.Result{}, fmt.Errorf("reconciling terraform: %w", err)
	}

	slog.Info(
		"updating agent pool status",
		"name",
		agentPool.Name,
	)
	applyAgentPool.Status = &AgentPoolStatus{
		Ready:  true,
		TFVars: tfVars,
	}
	if err := agentPoolClient.Apply(ctx, applyAgentPool, hz.WithApplyForce(true)); err != nil {
		return hz.Result{}, fmt.Errorf("applying agent pool: %w", err)
	}

	return hz.Result{}, nil
}
