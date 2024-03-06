package agentpool_test

import (
	"testing"
)

func TestAgentPool(t *testing.T) {
	// TODO: figure out a good test strategy because this depends on so much...
	// ctx := context.Background()
	// ts := server.Test(t, ctx)

	// vmssRecon := vmss.Reconciler{
	// 	Client: hz.NewClient(
	// 		ts.Conn,
	// 		hz.WithClientInternal(true),
	// 		hz.WithClientManager(vmss.ManagerName),
	// 	),
	// 	SubnetID:
	// "/subscriptions/12749df0-9a8e-44cd-889e-4740be851c13/resourceGroups/rg-default/providers/Microsoft.Network/virtualNetworks/default/subnets/default",
	// 	SubscriptionID: "12749df0-9a8e-44cd-889e-4740be851c13",
	// }
	// vmScaleSet := vmss.VMScaleSet{
	// 	ObjectMeta: hz.ObjectMeta{
	// 		Name:    "vmss1",
	// 		Account: "test",
	// 		Finalizers: &hz.Finalizers{
	// 			vmss.Finalizer,
	// 		},
	// 	},
	// 	Spec: &vmss.VMScaleSetSpec{
	// 		Location:          "swedencentral",
	// 		ResourceGroupName: "rg-default",
	// 		VMSize:            "Standard_DS1_v2",
	// 	},
	// }

	// t.Log("creating vmss", vmScaleSet.Name)
	// vmssResp, err := vmssRecon.CreateVMScaleSet(
	// 	ctx,
	// 	vmss.CreateVMScaleSetRequest{
	// 		Name:              vmScaleSet.Name,
	// 		Location:          vmScaleSet.Spec.Location,
	// 		ResourceGroupName: vmScaleSet.Spec.ResourceGroupName,
	// 		VMSize:            vmScaleSet.Spec.VMSize,
	// 	},
	// )
	// tu.AssertNoError(t, err)
	// t.Log("created vmss", vmssResp.ID)

	// t.Cleanup(func() {
	// 	t.Log("deleting vmss", vmScaleSet.Name)
	// 	err := vmssRecon.DeleteVMScaleSet(ctx, vmss.DeleteVMScaleSetRequest{
	// 		Name:              vmScaleSet.Name,
	// 		ResourceGroupName: vmScaleSet.Spec.ResourceGroupName,
	// 	})
	// 	tu.AssertNoError(t, err)
	// })

	// // resp, err := azClient.ListAgentPools(ctx,
	// // agentpool.ListAgentPoolsRequest{
	// // 	FilterName: "some-agent-pool",
	// // })
	// // tu.AssertNoError(t, err)

	// // if len(resp.AgentPools) == 0 {
	// // 	result, err := azClient.CreateAgentPool(
	// // 		ctx,
	// // 		agentpool.CreateAgentPoolRequest{
	// // 			Name:                "some-agent-pool",
	// // 			VMScaleSetID:
	// //
	// "/subscriptions/12749df0-9a8e-44cd-889e-4740be851c13/resourceGroups/rg-default/providers/Microsoft.Compute/virtualMachineScaleSets/vmss1",
	// // 			ProjectID:           "6d839fbd-d956-4e95-8a5c-d72e3315bcf2", //
	// 			ServiceConnectionID: "30d5d110-6ade-42b9-a016-a6010b8b4910",
	// // 		},
	// // 	)
	// // 	tu.AssertNoError(t, err)
	// // 	fmt.Println("RESULT: ", result)
	// // }

	// // err = azClient.DeleteAgentPool(ctx, agentpool.DeleteAgentPoolRequest{
	// // 	ID: *resp.AgentPools[0].Id,
	// // })
	// // tu.AssertNoError(t, err)

	// // t.Fatal("STOP")

	// projectCtlr, err := hz.StartController(
	// 	ctx,
	// 	ts.Conn,
	// 	hz.WithControllerFor(project.Project{}),
	// )
	// tu.AssertNoError(t, err)
	// t.Cleanup(func() {
	// 	_ = projectCtlr.Stop()
	// })

	// // Start vmss controller without reconciler.
	// vmssCtlr, err := hz.StartController(
	// 	ctx,
	// 	ts.Conn,
	// 	hz.WithControllerFor(vmss.VMScaleSet{}),
	// )
	// tu.AssertNoError(t, err)
	// t.Cleanup(func() {
	// 	_ = vmssCtlr.Stop()
	// })

	// apRecon := agentpool.Reconciler{
	// 	Client: hz.NewClient(
	// 		ts.Conn,
	// 		hz.WithClientInternal(true),
	// 		hz.WithClientManager(agentpool.ManagerName),
	// 	),
	// }

	// apCtlr, err := hz.StartController(
	// 	ctx,
	// 	ts.Conn,
	// 	hz.WithControllerFor(agentpool.AgentPool{}),
	// 	hz.WithControllerReconciler(&apRecon),
	// )
	// tu.AssertNoError(t, err)
	// t.Cleanup(func() {
	// 	_ = apCtlr.Stop()
	// })

	// client := hz.NewClient(
	// 	ts.Conn,
	// 	hz.WithClientInternal(true),
	// 	hz.WithClientDefaultManager(),
	// )
	// apClient := hz.ObjectClient[agentpool.AgentPool]{
	// 	Client: client,
	// }

	// // Create VMSS with status.
	// vmScaleSet.Status = &vmss.VMScaleSetStatus{
	// 	ID: vmssResp.ID,
	// }
	// err = client.Apply(ctx, hz.WithApplyObject(vmScaleSet))
	// tu.AssertNoError(t, err)

	// // Create Agent Pool
	// agentPool := agentpool.AgentPool{
	// 	ObjectMeta: hz.ObjectMeta{
	// 		Name:    "agentpool1",
	// 		Account: "test",
	// 		Finalizers: &hz.Finalizers{
	// 			agentpool.Finalizer,
	// 		},
	// 	},
	// 	Spec: &agentpool.AgentPoolSpec{
	// 		ProjectRef: agentpool.ProjectRef{
	// 			Name: "project1",
	// 		},
	// 		VMScaleSetRef: agentpool.VMScaleSetRef{
	// 			Name: vmScaleSet.Name,
	// 		},
	// 	},
	// }

	// if err := apClient.Apply(ctx, agentPool); err != nil {
	// 	t.Fatal("applying agentpool1: ", err)
	// }
	// {
	// 	timeout := time.After(time.Second * 120)
	// 	done := make(chan struct{})
	// 	watcher, err := hz.StartWatcher(
	// 		ctx,
	// 		ts.Conn,
	// 		hz.WithWatcherFor(agentPool),
	// 		hz.WithWatcherFn(
	// 			func(event hz.Event) (hz.Result, error) {
	// 				var watchAP agentpool.AgentPool
	// 				if err := json.Unmarshal(event.Data, &watchAP); err != nil {
	// 					return hz.Result{}, fmt.Errorf(
	// 						"unmarshalling greeting: %w",
	// 						err,
	// 					)
	// 				}
	// 				if watchAP.Status == nil {
	// 					return hz.Result{}, nil
	// 				}
	// 				if watchAP.Status.Ready {
	// 					close(done)
	// 				}
	// 				return hz.Result{}, nil
	// 			},
	// 		),
	// 	)
	// 	if err != nil {
	// 		t.Fatal("starting vmss watcher: ", err)
	// 	}
	// 	t.Cleanup(func() {
	// 		watcher.Close()
	// 	})

	// 	select {
	// 	case <-timeout:
	// 		t.Fatal("timed out waiting for vmss")
	// 	case <-done:
	// 		watcher.Close()
	// 	}
	// }

	// err = apClient.Delete(ctx, agentPool)
	// tu.AssertNoError(t, err)
	// {
	// 	timeout := time.After(time.Second * 120)
	// 	done := make(chan struct{})
	// 	watcher, err := hz.StartWatcher(
	// 		ctx,
	// 		ts.Conn,
	// 		hz.WithWatcherFor(agentPool),
	// 		hz.WithWatcherFn(
	// 			func(event hz.Event) (hz.Result, error) {
	// 				if event.Operation == hz.EventOperationPurge {
	// 					close(done)
	// 				}
	// 				return hz.Result{}, nil
	// 			},
	// 		),
	// 	)
	// 	if err != nil {
	// 		t.Fatal("starting agentpool watcher: ", err)
	// 	}
	// 	defer watcher.Close()

	// 	select {
	// 	case <-timeout:
	// 		t.Fatal("timed out waiting for agentpool")
	// 	case <-done:
	// 	}
	// }
}
