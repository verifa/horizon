package vmss

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*VMScaleSet)(nil)

type VMScaleSet struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *VMScaleSetSpec   `json:"spec,omitempty" cue:",opt"`
	Status *VMScaleSetStatus `json:"status,omitempty" cue:",opt"`
}

func (a VMScaleSet) ObjectGroup() string {
	return "azuredevops"
}

func (a VMScaleSet) ObjectVersion() string {
	return "v1"
}

func (a VMScaleSet) ObjectKind() string {
	return "VMScaleSet"
}

type VMScaleSetSpec struct {
	Location          string `json:"location,omitempty" cue:""`
	ResourceGroupName string `json:"resource_group_name,omitempty" cue:""`
	VMSize            string `json:"vm_size,omitempty" cue:""`
}

type VMScaleSetStatus struct {
	Ready bool   `json:"ready,omitempty"`
	ID    string `json:"id,omitempty"`
}
