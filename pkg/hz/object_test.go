package hz_test

import (
	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Objecter) = (*DummyObject)(nil)

type DummyObject struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   struct{} `json:"spec,omitempty" cue:""`
	Status struct{} `json:"status,omitempty"`
}

func (o DummyObject) ObjectKind() string {
	return "DummyObject"
}

func (o DummyObject) ObjectGroup() string {
	return "DummyGroup"
}

type ChildObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec struct{} `json:"spec,omitempty" cue:",opt"`
}

func (o ChildObject) ObjectKind() string {
	return "ChildObject"
}

func (o ChildObject) ObjectGroup() string {
	return "ChildGroup"
}
