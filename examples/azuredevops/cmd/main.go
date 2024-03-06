package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/examples/azuredevops"
	"github.com/verifa/horizon/examples/azuredevops/agentpool"
	"github.com/verifa/horizon/examples/azuredevops/project"
	"github.com/verifa/horizon/examples/azuredevops/vmss"
	"github.com/verifa/horizon/pkg/hz"
)

func main() {
	if err := run(); err != nil {
		slog.Error("running", "error", err)
		os.Exit(1)
	}
}

func run() error {
	conn, err := nats.Connect(
		nats.DefaultURL,
		nats.UserCredentials("nats.creds"),
	)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	vmssRecon := vmss.Reconciler{
		SubscriptionID: "12749df0-9a8e-44cd-889e-4740be851c13",
		SubnetID:       "/subscriptions/12749df0-9a8e-44cd-889e-4740be851c13/resourceGroups/rg-default/providers/Microsoft.Network/virtualNetworks/default/subnets/default",
		Client: hz.NewClient(
			conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(vmss.ManagerName),
		),
	}
	vmssCtlr, err := hz.StartController(
		ctx,
		conn,
		hz.WithControllerFor(vmss.VMScaleSet{}),
		hz.WithControllerReconciler(&vmssRecon),
	)
	if err != nil {
		return fmt.Errorf("starting vmss controller: %w", err)
	}
	defer func() {
		_ = vmssCtlr.Stop()
	}()

	projectRecon := project.Reconciler{
		Client: hz.NewClient(
			conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(project.ManagerName),
		),
	}
	projectCtlr, err := hz.StartController(
		ctx,
		conn,
		hz.WithControllerFor(project.Project{}),
		hz.WithControllerReconciler(&projectRecon),
	)
	if err != nil {
		return fmt.Errorf("starting project controller: %w", err)
	}
	defer func() {
		_ = projectCtlr.Stop()
	}()

	agentPoolRecon := agentpool.Reconciler{
		Client: hz.NewClient(
			conn,
			hz.WithClientInternal(true),
			hz.WithClientManager(agentpool.ManagerName),
		),
	}
	agentPoolCtlr, err := hz.StartController(
		ctx,
		conn,
		hz.WithControllerFor(agentpool.AgentPool{}),
		hz.WithControllerReconciler(&agentPoolRecon),
	)
	if err != nil {
		return fmt.Errorf("starting agent pool controller: %w", err)
	}
	defer func() {
		_ = agentPoolCtlr.Stop()
	}()

	portalHandler := azuredevops.PortalHandler{
		Conn: conn,
	}
	router := portalHandler.Router()
	portal, err := hz.StartPortal(ctx, conn, azuredevops.Portal, router)
	if err != nil {
		return fmt.Errorf("start portal: %w", err)
	}
	defer func() {
		_ = portal.Stop()
	}()

	<-ctx.Done()
	return nil
}
