package greetings

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Greeting)(nil)

type Greeting struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *GreetingSpec   `json:"spec,omitempty" cue:""`
	Status *GreetingStatus `json:"status,omitempty"`
}

func (s Greeting) ObjectGroup() string {
	return "greetings"
}

func (s Greeting) ObjectVersion() string {
	return "v1"
}

func (s Greeting) ObjectKind() string {
	return "Greeting"
}

// GreetingSpec defines the desired state of Greeting.
type GreetingSpec struct {
	// Name of the person to greet.
	Name string `json:"name,omitempty" cue:""`
}

// GreetingStatus defines the observed state of Greeting.
type GreetingStatus struct {
	// Ready indicates whether the greeting is ready.
	Ready bool `json:"ready"`
	// Error is the error message if the greeting failed.
	Error string `json:"error,omitempty" cue:",opt"`
	// Response is the response of the greeting.
	Response string `json:"response,omitempty" cue:",opt"`
}
