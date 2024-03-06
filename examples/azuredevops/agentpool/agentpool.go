package agentpool

import (
	"github.com/verifa/horizon/examples/azuredevops/agentpool/tf"
	"github.com/verifa/horizon/pkg/hz"
)

type AgentPool struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *AgentPoolSpec   `json:"spec,omitempty" cue:",opt"`
	Status *AgentPoolStatus `json:"status,omitempty" cue:",opt"`
}

func (a AgentPool) ObjectGroup() string {
	return "azuredevops"
}

func (a AgentPool) ObjectVersion() string {
	return "v1"
}

func (a AgentPool) ObjectKind() string {
	return "AgentPool"
}

type AgentPoolSpec struct {
	VMScaleSetRef VMScaleSetRef `json:"vmscaleset_ref,omitempty" cue:""`
	ProjectRef    ProjectRef    `json:"project_ref,omitempty" cue:""`
}

type VMScaleSetRef struct {
	Name string `json:"name,omitempty" cue:""`
}

type ProjectRef struct {
	Name string `json:"name,omitempty" cue:""`
}

type AgentPoolStatus struct {
	Ready  bool    `json:"ready,omitempty"`
	TFVars tf.Vars `json:"tfvars,omitempty"`
}
