package tf

import "github.com/verifa/horizon/examples/azuredevops/terra"

type Vars struct {
	VMScaleSet *VMScaleSet `json:"vm_scale_set,omitempty"`
}

type VMScaleSet struct {
	Name              string `json:"name,omitempty"`
	ResourceGroupName string `json:"resource_group_name,omitempty"`
	Location          string `json:"location,omitempty"`
	Sku               string `json:"sku,omitempty"`
	SubnetID          string `json:"subnet_id,omitempty"`
}

//
// Outputs
//

var _ terra.Outputer = (*OutputVMScaleSet)(nil)

type OutputVMScaleSet struct {
	ID string `json:"id"`
}

func (p *OutputVMScaleSet) Name() string {
	return "vmscaleset"
}

func (p *OutputVMScaleSet) Resource() string {
	return "azurerm_linux_virtual_machine_scale_set.this"
}
