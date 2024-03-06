package project

import (
	"context"
	"fmt"
	"testing"

	"github.com/verifa/horizon/examples/azuredevops/project/tf"
	"github.com/verifa/horizon/examples/azuredevops/terra"
	"github.com/verifa/horizon/pkg/hz"
)

func TestTF(t *testing.T) {
	ctx := context.Background()

	project := &Project{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "test-project",
		},
	}
	projectOutput := tf.OutputProject{}

	result, err := terra.Reconcile(
		ctx,
		terra.WithWorkdir("tf_tmp"),
		terra.WithFS(terraformFS),
		terra.WithFSSub("tf"),
		// terra.WithDestroy(true),
		// terra.WithSkipApply(true),

		terra.WithTFVars(tf.DefaultTFVars),

		terra.WithBackend(terra.BackendAzureRM{
			ResourceGroupName:  "rg-default",
			StorageAccountName: "verifahorizon",
			ContainerName:      "tfstate",
			Key:                hz.KeyFromObject(project),
		}),
		terra.WithOutputs(&projectOutput),
	)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(result)
	fmt.Println(projectOutput)
}
