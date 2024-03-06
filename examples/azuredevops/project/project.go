package project

import (
	"github.com/verifa/horizon/pkg/hz"
)

var _ hz.Objecter = (*Project)(nil)

type Project struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *ProjectSpec   `json:"spec,omitempty" cue:",opt"`
	Status *ProjectStatus `json:"status,omitempty" cue:",opt"`
}

func (a Project) ObjectGroup() string {
	return "azuredevops"
}

func (a Project) ObjectVersion() string {
	return "v1"
}

func (a Project) ObjectKind() string {
	return "Project"
}

type ProjectSpec struct{}

type ProjectStatus struct {
	Ready bool `json:"ready,omitempty"`
	// ID is the ID of the project in Azure DevOps.
	ID                  string `json:"id,omitempty"`
	ServiceConnectionID string `json:"service_connection_id,omitempty"`
}
