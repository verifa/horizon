package hz_test

import (
	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Objecter) = (*DummyObject)(nil)

type DummyObject struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   DummyObjectSpec   `json:"spec,omitempty" cue:""`
	Status DummyObjectStatus `json:"status,omitempty"`
}

func (o DummyObject) ObjectKind() string {
	return "DummyObject"
}

type DummyObjectSpec struct {
	SmallNumber int `json:"number,omitempty" cue:",opt"`
}

type DummyObjectStatus struct{}

type ChildObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec ChildObjectSpec `json:"spec,omitempty" cue:",opt"`
}

func (o ChildObject) ObjectKind() string {
	return "ChildObject"
}

type ChildObjectSpec struct {
	Field string `json:"field,omitempty"`
}
