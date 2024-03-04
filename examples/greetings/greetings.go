package greetings

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Greeting)(nil)

type Greeting struct {
	hz.ObjectMeta `json:"metadata"`

	Spec   *GreetingSpec   `json:"spec,omitempty"`
	Status *GreetingStatus `json:"status,omitempty"`
}

func (s Greeting) ObjectGroup() string {
	return "hz-examples"
}

func (s Greeting) ObjectVersion() string {
	return "v1"
}

func (s Greeting) ObjectKind() string {
	return "Greeting"
}

type GreetingSpec struct {
	// Name of the person to greet.
	Name *string `json:"name,omitempty" cue:""`
}

type GreetingStatus struct {
	Ready          bool        `json:"ready"`
	Phase          StatusPhase `json:"phase"`
	FailureReason  string      `json:"failureReason"`
	FailureMessage string      `json:"failureMessage"`

	Response string `json:"response,omitempty"`
}

type StatusPhase string

const (
	StatusPhasePending   StatusPhase = "Pending"
	StatusPhaseCompleted StatusPhase = "Completed"
	StatusPhaseFailed    StatusPhase = "Failed"
)
