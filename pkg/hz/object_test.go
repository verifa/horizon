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

func (o DummyObject) ObjectGroup() string {
	return "DummyGroup"
}

func (o DummyObject) ObjectVersion() string {
	return "v1"
}

func (o DummyObject) ObjectKind() string {
	return "DummyObject"
}

type ChildObject struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec struct{} `json:"spec,omitempty" cue:",opt"`
}

func (o ChildObject) ObjectGroup() string {
	return "ChildGroup"
}

func (o ChildObject) ObjectVersion() string {
	return "v1"
}

func (o ChildObject) ObjectKind() string {
	return "ChildObject"
}
