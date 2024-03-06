package project_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verifa/horizon/examples/azuredevops/project"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

func TestProject(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)

	recon := project.Reconciler{
		Client: hz.NewClient(
			ts.Conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(project.ManagerName),
		),
	}

	ctlr, err := hz.StartController(
		ctx,
		ts.Conn,
		hz.WithControllerFor(project.Project{}),
		hz.WithControllerReconciler(&recon),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	adoProject := project.Project{
		ObjectMeta: hz.ObjectMeta{
			Name:    "project1",
			Account: "test",
			Finalizers: &hz.Finalizers{
				project.Finalizer,
			},
		},
		Spec: &project.ProjectSpec{},
	}

	client := hz.ObjectClient[project.Project]{
		Client: hz.NewClient(
			ts.Conn,
			hz.WithClientInternal(true),
			hz.WithClientDefaultManager(),
		),
	}
	err = client.Apply(ctx, adoProject)
	tu.AssertNoError(t, err)

	{
		timeout := time.After(time.Second * 120)
		done := make(chan struct{})
		watcher, err := hz.StartWatcher(
			ctx,
			ts.Conn,
			hz.WithWatcherFor(adoProject),
			hz.WithWatcherFn(
				func(event hz.Event) (hz.Result, error) {
					var watchProject project.Project
					if err := json.Unmarshal(event.Data, &watchProject); err != nil {
						return hz.Result{}, fmt.Errorf(
							"unmarshalling greeting: %w",
							err,
						)
					}
					if watchProject.Status == nil {
						return hz.Result{}, nil
					}
					s := watchProject.Status
					if s.Ready && s.ID != "" && s.ServiceConnectionID != "" {
						close(done)
					}
					return hz.Result{}, nil
				},
			),
		)
		if err != nil {
			t.Fatal("starting project watcher: ", err)
		}
		t.Cleanup(func() {
			watcher.Close()
		})

		select {
		case <-timeout:
			t.Fatal("timed out waiting for project")
		case <-done:
			watcher.Close()
		}
	}

	err = client.Delete(ctx, adoProject)
	tu.AssertNoError(t, err)
	{
		timeout := time.After(time.Second * 120)
		done := make(chan struct{})
		watcher, err := hz.StartWatcher(
			ctx,
			ts.Conn,
			hz.WithWatcherFor(adoProject),
			hz.WithWatcherFn(
				func(event hz.Event) (hz.Result, error) {
					if event.Operation == hz.EventOperationPurge {
						close(done)
					}
					return hz.Result{}, nil
				},
			),
		)
		if err != nil {
			t.Fatal("starting project watcher: ", err)
		}
		defer watcher.Close()

		select {
		case <-timeout:
			t.Fatal("timed out waiting for project")
		case <-done:
		}
	}
}
