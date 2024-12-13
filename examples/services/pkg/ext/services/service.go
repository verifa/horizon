package services

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Service)(nil)

type Service struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *ServiceSpec   `json:"spec,omitempty" cue:""`
	Status *ServiceStatus `json:"status,omitempty" cue:",opt"`
}

func (s Service) ObjectGroup() string {
	return "services"
}

func (s Service) ObjectVersion() string {
	return "v1"
}

func (s Service) ObjectKind() string {
	return "Service"
}

// ServiceSpec defines the desired state (i.e. inputs).
type ServiceSpec struct {
	// Host is the fully qualified domain name of the service.
	Host *string `json:"host" cue:""`
	// Image is the container image to run.
	Image *string `json:"image,omitempty" cue:""`
	// Command is the command to run in the container.
	Command []string `json:"command,omitempty" cue:",opt"`
	// Args is the arguments to the command.
	Args []string `json:"args,omitempty" cue:",opt"`
	// Env is the environment variables to set in the container.
	Env map[string]string `json:"env,omitempty" cue:",opt"`
	// Resources defines the resource requirements (requests and limits).
	Resources *Resources `json:"resources,omitempty" cue:",opt"`
}

type Resources struct {
	Requests ResourceQuantity `json:"requests,omitempty" cue:",opt"`
	Limits   ResourceQuantity `json:"limits,omitempty" cue:",opt"`
}

type ResourceQuantity struct {
	CPU    string `json:"cpu,omitempty" cue:",opt"`
	Memory string `json:"memory,omitempty" cue:",opt"`
}

// ServiceStatus defines the observed state (i.e. outputs as set by the
// controller).
type ServiceStatus struct {
	Ready bool    `json:"ready,omitempty" cue:",opt"`
	Error *string `json:"error,omitempty" cue:",opt"`
}
