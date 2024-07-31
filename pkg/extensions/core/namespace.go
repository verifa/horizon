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

// Override ObjectNamespace because namespaces can only exist in the root
// namespace.
func (a Namespace) ObjectNamespace() string {
	return hz.RootNamespace
}

type NamespaceSpec struct{}

type NamespaceStatus struct{}
