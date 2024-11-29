package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/verifa/horizon/examples/services/pkg/ext/services"
	"github.com/verifa/horizon/pkg/controller"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/hztest"
	"github.com/verifa/horizon/pkg/server"
)

func TestService(t *testing.T) {
	ctx := context.Background()
	// Create a test server which includes the core of Horizon.
	ts := server.Test(t, ctx)
	client := hz.NewClient(
		ts.Conn,
		hz.WithClientInternal(true),
		hz.WithClientManager("ctlr-counter"),
	)
	serviceClient := hz.ObjectClient[services.Service]{Client: client}

	//
	// Setup counter controller with validator and reconciler.
	//
	validr := services.Validator{}
	recon := services.Reconciler{
		Client: serviceClient,
	}
	ctlr, err := controller.StartController(
		ctx,
		ts.Conn,
		controller.WithControllerFor(services.Service{}),
		controller.WithControllerValidator(&validr),
		controller.WithControllerReconciler(&recon),
	)
	if err != nil {
		t.Fatal("starting service controller: ", err)
	}
	defer ctlr.Stop()

	//
	// Apply a counter object.
	//
	service := services.Service{
		ObjectMeta: hz.ObjectMeta{
			Namespace: "test",
			Name:      "test",
		},
		Spec: &services.ServiceSpec{
			Host:  hz.P("test.horizon.verifa.io"),
			Image: hz.P("nginx"),
		},
	}
	_, err = serviceClient.Apply(ctx, service)
	if err != nil {
		t.Fatal("applying service: ", err)
	}

	//
	// Verify that the controller reconciles the object.
	//
	// Watch until the service is ready.
	// If the timeout is reached, the test fails.
	//
	hztest.WatchWaitUntil(
		t,
		ctx,
		ts.Conn,
		time.Second*5,
		service,
		func(service services.Service) bool {
			if service.Status == nil {
				return false
			}
			return service.Status.Ready
		},
	)
}
