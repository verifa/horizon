package agentpool

import (
	"context"
	"fmt"
	"testing"

	"github.com/verifa/horizon/examples/azuredevops/agentpool/tf"
	"github.com/verifa/horizon/examples/azuredevops/terra"
	"github.com/verifa/horizon/pkg/hz"
)

func TestTF(t *testing.T) {
	ctx := context.Background()

	agentPool := &AgentPool{
		ObjectMeta: hz.ObjectMeta{
			Account: "test",
			Name:    "test-pool",
		},
	}

	tfVars := tf.Vars{
		AgentPool: &tf.AgentPool{
			Name:                "test-pool",
			ProjectID:           "098a113b-af5c-4af9-b487-ce157ecb9db8",
			VMScaleSetID:        "/subscriptions/12749df0-9a8e-44cd-889e-4740be851c13/resourceGroups/rg-default/providers/Microsoft.Compute/virtualMachineScaleSets/test",
			ServiceConnectionID: "dac282de-2ce4-4428-8a5c-504530a9e96e",
		},
	}

	result, err := terra.Reconcile(
		ctx,
		terra.WithWorkdir("tf_tmp"),
		terra.WithFS(terraformFS),
		terra.WithFSSub("tf"),
		terra.WithDestroy(true),
		// terra.WithSkipApply(true),

		terra.WithTFVars(tfVars),

		terra.WithBackend(terra.BackendAzureRM{
			ResourceGroupName:  "rg-default",
			StorageAccountName: "verifahorizon",
			ContainerName:      "tfstate",
			Key:                hz.KeyFromObject(agentPool),
		}),
		// terra.WithOutputs(&projectOutput),
	)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(result)
	// fmt.Println(projectOutput)
}
