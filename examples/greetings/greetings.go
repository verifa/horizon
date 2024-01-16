package greetings

import "github.com/verifa/horizon/pkg/hz"

type Greeting struct {
	hz.ObjectMeta `json:"metadata"`

	Spec   GreetingSpec   `json:"spec"`
	Status GreetingStatus `json:"status"`
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
}

type StatusPhase string

const (
	StatusPhasePending   StatusPhase = "Pending"
	StatusPhaseCompleted StatusPhase = "Completed"
	StatusPhaseFailed    StatusPhase = "Failed"
)
