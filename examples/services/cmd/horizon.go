package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/examples/services/pkg/ext/services"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/controller"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/gateway"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/server"

	cloudrun "google.golang.org/api/run/v1"
)

func main() {
	if err := run(); err != nil {
		slog.Error("running", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	//
	// Connect to Google Cloud Run API.
	//
	_, err := cloudrun.NewService(ctx)
	if err != nil {
		return fmt.Errorf("connect to Google Cloud Run API: %w", err)
	}
	//
	// Start Horizon embedded server.
	//
	s, err := server.Start(
		ctx,
		server.WithDevMode(),
		server.WithAuthOptions(auth.WithAdminGroups("admin")),
		server.WithGatewayOptions(
			gateway.WithOIDCConfig(gateway.OIDCConfig{
				Issuer:       "https://accounts.google.com",
				ClientID:     "50561335587-qp0rcctibj88mcpg13fh9ecq2mlb9faq.apps.googleusercontent.com",
				ClientSecret: "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",
				RedirectURL:  "http://localhost:9999/auth/callback",
				Scopes: []string{
					"openid",
					"profile",
					"email",
				},
			}),
		),
	)
	if err != nil {
		return err
	}
	defer s.Close()
	slog.Info("horizon server started")

	//
	// Start Horizon extensions (controllers, portals, etc.).
	//
	validator := services.Validator{}
	reconciler := services.Reconciler{
		Client: hz.ObjectClient[services.Service]{
			Client: hz.NewClient(
				s.Conn,
				hz.WithClientInternal(true),
				hz.WithClientManager("ctlr-services"),
			),
		},
	}
	ctlr, err := controller.StartController(
		ctx,
		s.Conn,
		controller.WithControllerFor(services.Service{}),
		controller.WithControllerReconciler(&reconciler),
		controller.WithControllerValidator(&validator),
	)
	if err != nil {
		return fmt.Errorf("start controller: %w", err)
	}
	defer func() {
		_ = ctlr.Stop()
	}()

	portalHandler := services.PortalHandler{
		Conn: s.Conn,
	}
	router := portalHandler.Router()
	portal, err := hz.StartPortal(ctx, s.Conn, services.Portal, router)
	if err != nil {
		return fmt.Errorf("start portal: %w", err)
	}
	defer func() {
		_ = portal.Stop()
	}()

	if err := setupDefaultRBAC(ctx, s.Conn); err != nil {
		return fmt.Errorf("setup default RBAC: %w", err)
	}

	if err := createDemoService(ctx, s.Conn); err != nil {
		return fmt.Errorf("create demo service: %w", err)
	}

	// Wait for interrupt signal.
	<-ctx.Done()
	// Stop listening for interrupts so that a second interrupt will force
	// shutdown.
	stop()
	slog.Info(
		"interrupt received, shutting down horizon server",
	)
	return nil
}

func setupDefaultRBAC(ctx context.Context, conn *nats.Conn) error {
	client := hz.NewClient(conn, hz.WithClientInternal(true))
	authenticatedRole := auth.Role{
		ObjectMeta: hz.ObjectMeta{
			Name:      "authenticated-users-role",
			Namespace: hz.NamespaceRoot,
		},
		Spec: auth.RoleSpec{
			Allow: []auth.Rule{
				{
					Group: hz.P("*"),
					Kind:  hz.P("Namespace"),
					Verbs: []auth.Verb{auth.VerbRead, auth.VerbCreate},
				},
				{
					Group: hz.P("*"),
					Kind:  hz.P("Service"),
					Verbs: []auth.Verb{auth.VerbRead, auth.VerbAll},
				},
			},
		},
	}
	authenticatedRoleBinding := auth.RoleBinding{
		ObjectMeta: hz.ObjectMeta{
			Name:      "authenticated-users-role-binding",
			Namespace: hz.NamespaceRoot,
		},
		Spec: auth.RoleBindingSpec{
			RoleRef: auth.RoleRefFromRole(authenticatedRole),
			Subjects: []auth.Subject{
				{
					Kind: "Group",
					Name: auth.GroupSystemAuthenticated,
				},
			},
		},
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(authenticatedRole)); err != nil {
		return fmt.Errorf("apply namespace role: %w", err)
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(authenticatedRoleBinding)); err != nil {
		return fmt.Errorf("apply namespace role binding: %w", err)
	}
	return nil
}

func createDemoService(ctx context.Context, conn *nats.Conn) error {
	client := hz.NewClient(conn, hz.WithClientInternal(true))
	ns := core.Namespace{
		ObjectMeta: hz.ObjectMeta{
			Name:      "demo",
			Namespace: hz.NamespaceRoot,
		},
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(ns)); err != nil {
		return fmt.Errorf("apply namespace: %w", err)
	}
	service := services.Service{
		ObjectMeta: hz.ObjectMeta{
			Namespace: ns.Name,
			Name:      "demo-prod",
		},
		Spec: &services.ServiceSpec{
			Host:  hz.P("demo.horizon.xyz"),
			Image: hz.P("horizon-demo:123456"),
		},
	}
	if _, err := client.Apply(ctx, hz.WithApplyObject(service)); err != nil {
		return fmt.Errorf("apply service: %w", err)
	}
	return nil
}
