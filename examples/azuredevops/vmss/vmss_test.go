package vmss_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/verifa/horizon/examples/azuredevops/vmss"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"
	tu "github.com/verifa/horizon/pkg/testutil"
)

type Whatever string

func TestVMSS(t *testing.T) {
	ctx := context.Background()
	ts := server.Test(t, ctx)

	recon := vmss.Reconciler{
		SubnetID:       "/subscriptions/12749df0-9a8e-44cd-889e-4740be851c13/resourceGroups/rg-default/providers/Microsoft.Network/virtualNetworks/default/subnets/default",
		SubscriptionID: "12749df0-9a8e-44cd-889e-4740be851c13",
		Client: hz.NewClient(
			ts.Conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(vmss.ManagerName),
		),
	}

	ctlr, err := hz.StartController(
		ctx,
		ts.Conn,
		hz.WithControllerFor(vmss.VMScaleSet{}),
		hz.WithControllerReconciler(&recon),
	)
	tu.AssertNoError(t, err)
	t.Cleanup(func() {
		_ = ctlr.Stop()
	})

	client := hz.ObjectClient[vmss.VMScaleSet]{
		Client: hz.NewClient(
			ts.Conn,
			hz.WithClientInternal(true),
			hz.WithClientDefaultManager(),
		),
	}

	vmScaleSet := vmss.VMScaleSet{
		ObjectMeta: hz.ObjectMeta{
			Name:    "vmss1",
			Account: "test",
			Finalizers: &hz.Finalizers{
				vmss.Finalizer,
			},
		},
		Spec: &vmss.VMScaleSetSpec{
			Location:          "swedencentral",
			ResourceGroupName: "rg-default",
			VMSize:            "Standard_DS1_v2",
		},
	}

	if err := client.Apply(ctx, vmScaleSet); err != nil {
		t.Fatal("applying vmss1: ", err)
	}

	{
		timeout := time.After(time.Second * 120)
		done := make(chan struct{})
		watcher, err := hz.StartWatcher(
			ctx,
			ts.Conn,
			hz.WithWatcherFor(vmScaleSet),
			hz.WithWatcherFn(
				func(event hz.Event) (hz.Result, error) {
					var watchVMSS vmss.VMScaleSet
					if err := json.Unmarshal(event.Data, &watchVMSS); err != nil {
						return hz.Result{}, fmt.Errorf(
							"unmarshalling greeting: %w",
							err,
						)
					}
					if watchVMSS.Status == nil {
						return hz.Result{}, nil
					}
					if watchVMSS.Status.ID != "" {
						close(done)
					}
					return hz.Result{}, nil
				},
			),
		)
		if err != nil {
			t.Fatal("starting vmss watcher: ", err)
		}
		t.Cleanup(func() {
			watcher.Close()
		})

		select {
		case <-timeout:
			t.Fatal("timed out waiting for vmss")
		case <-done:
			watcher.Close()
		}
	}

	err = client.Delete(ctx, vmScaleSet)
	tu.AssertNoError(t, err)
	{
		timeout := time.After(time.Second * 120)
		done := make(chan struct{})
		watcher, err := hz.StartWatcher(
			ctx,
			ts.Conn,
			hz.WithWatcherFor(vmScaleSet),
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
			t.Fatal("starting vmss watcher: ", err)
		}
		defer watcher.Close()

		select {
		case <-timeout:
			t.Fatal("timed out waiting for vmss")
		case <-done:
		}
	}
}
