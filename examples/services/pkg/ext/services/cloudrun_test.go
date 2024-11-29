package services_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"google.golang.org/api/option"
	cloudrun "google.golang.org/api/run/v1"
)

func TestCludRun(t *testing.T) {
	err := do()
	if err != nil {
		t.Fatal(err)
	}
}

func do() error {
	ctx := context.Background()
	region := "europe-north1"
	runService, err := cloudrun.NewService(
		ctx,
		option.WithEndpoint(
			fmt.Sprintf("https://%s-run.googleapis.com", region),
		),
	)
	if err != nil {
		return fmt.Errorf("connect to Google Cloud Run API: %w", err)
	}
	serviceURI := fmt.Sprintf(
		"namespaces/%s/services/%s",
		"verifa-website",
		"prod-website-service",
		// europe-north1/prod-website-service
	)
	svc, err := runService.Namespaces.Services.Get(serviceURI).
		Context(ctx).
		Do()
	if err != nil {
		return fmt.Errorf("get service: %w", err)
	}
	// TODO: check conditions.
	// svc.Status.Conditions
	spew.Dump(svc)
	return nil
}
