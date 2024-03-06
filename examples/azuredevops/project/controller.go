package project

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/verifa/horizon/examples/azuredevops/project/tf"
	"github.com/verifa/horizon/examples/azuredevops/terra"
	"github.com/verifa/horizon/pkg/hz"
)

//go:embed tf/*.tf
var terraformFS embed.FS

const (
	ManagerName = "ctlr-ado-project"
	Finalizer   = "azuredevops/project"
)

var _ hz.Reconciler = (*Reconciler)(nil)

type Reconciler struct {
	Client hz.Client
}

func (r *Reconciler) Reconcile(
	ctx context.Context,
	req hz.Request,
) (hz.Result, error) {
	projectClient := hz.ObjectClient[Project]{
		Client: r.Client,
	}

	adoProject, err := projectClient.Get(ctx, hz.WithGetKey(req.Key))
	if err != nil {
		return hz.Result{}, hz.IgnoreNotFound(err)
	}

	applyADOProject, err := hz.ExtractManagedFields(adoProject, ManagerName)
	if err != nil {
		return hz.Result{}, fmt.Errorf("extracting managed fields: %w", err)
	}

	slog.Info(
		"reconciling ADO Project",
		"spec",
		adoProject.Spec,
		"status",
		adoProject.Status,
	)

	tfVars := tf.Vars{
		Project: &tf.Project{
			Name:             adoProject.Name,
			Description:      "TODO: made by horizon",
			Visibility:       "private",
			VersionControl:   "Git",
			WorkItemTemplate: "Basic",
		},
		Subscription: &tf.AzureSubscription{
			ID:   "12749df0-9a8e-44cd-889e-4740be851c13",
			Name: "verifa-main",
		},
		Application: &tf.AzureADApplication{
			ClientID: "2a57f5af-ba13-481d-a17a-425c16dec0a6",
			TenantID: "449f7bd6-b339-4333-8b85-cd0b8fc37aa6",
		},
	}

	reconcileOptions := []terra.ReconcileOption{
		terra.WithFS(terraformFS),
		terra.WithFSSub("tf"),
		terra.WithBackend(terra.BackendAzureRM{
			ResourceGroupName:  "rg-default",
			StorageAccountName: "verifahorizon",
			ContainerName:      "tfstate",
			Key:                hz.KeyFromObject(req.Key),
		}),
		terra.WithTFVars(tfVars),
		terra.WithWorkdir(
			filepath.Join(
				os.TempDir(),
				"terraform",
				hz.KeyFromObject(req.Key),
			),
		),
	}

	// Handle deletion.
	if adoProject.ObjectMeta.DeletionTimestamp.IsPast() {
		slog.Info(
			"azure devops project deletion requested",
			"name",
			adoProject.Name,
			"status",
			adoProject.Status,
			"finalizers",
			adoProject.Finalizers,
		)
		if !adoProject.Finalizers.Contains(Finalizer) {
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
		if applyADOProject.Status == nil {
			return hz.Result{}, nil
		}
		slog.Info("project deleted", "name", adoProject.Name)
		// Remove finalizers and status, and apply.
		applyADOProject.Finalizers = &hz.Finalizers{}
		applyADOProject.Status = nil
		if err := projectClient.Apply(ctx, applyADOProject, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying ADO project: %w", err)
		}
		return hz.Result{}, nil
	}

	tfProjectOutput := tf.OutputProject{}
	tfServiceConnectionOutput := tf.OutputServiceConnection{}
	reconcileOptions = append(
		reconcileOptions,
		terra.WithOutputs(&tfProjectOutput, &tfServiceConnectionOutput),
	)

	_, err = terra.Reconcile(
		ctx,
		reconcileOptions...,
	)
	if err != nil {
		// Set status to indicate an error.
		if applyADOProject.Status == nil {
			applyADOProject.Status = &ProjectStatus{
				Ready: false,
			}
		}
		applyADOProject.Status.Ready = false
		if err := projectClient.Apply(ctx, applyADOProject, hz.WithApplyForce(true)); err != nil {
			return hz.Result{}, fmt.Errorf("applying ADO project: %w", err)
		}
		return hz.Result{}, fmt.Errorf("reconciling terraform: %w", err)
	}

	applyADOProject.Status = &ProjectStatus{
		Ready:               true,
		ID:                  tfProjectOutput.ID,
		ServiceConnectionID: tfServiceConnectionOutput.ID,
	}
	if err := projectClient.Apply(ctx, applyADOProject, hz.WithApplyForce(true)); err != nil {
		return hz.Result{}, fmt.Errorf("applying ADO project: %w", err)
	}

	return hz.Result{}, nil
}
