package core

import (
	"github.com/verifa/horizon/pkg/hz"
)

const (
	ObjectGroup   = "core"
	ObjectVersion = "v1"

	ObjectKindNamespace = "Namespace"
)

type Namespace struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   *NamespaceSpec   `json:"spec,omitempty"`
	Status *NamespaceStatus `json:"status,omitempty"`
}

func (a Namespace) ObjectGroup() string {
	return ObjectGroup
}

func (a Namespace) ObjectVersion() string {
	return ObjectVersion
}

func (a Namespace) ObjectKind() string {
	return "Namespace"
}

type NamespaceSpec struct{}

type NamespaceStatus struct{}
