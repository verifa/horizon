package auth

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*RoleBinding)(nil)

type RoleBinding struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec RoleBindingSpec `json:"spec,omitempty" cue:""`
}

func (RoleBinding) ObjectVersion() string {
	return "v1"
}

func (RoleBinding) ObjectGroup() string {
	return "core"
}

func (RoleBinding) ObjectKind() string {
	return "RoleBinding"
}

type RoleBindingSpec struct {
	// RoleRef is the reference to the Role which the RoleBinding will bind.
	RoleRef RoleRef `json:"roleRef" cue:""`
	// Subjects is the list of subjects that should have this Role.
	Subjects []Subject `json:"subjects" cue:""`
}

type RoleRef struct {
	// Group is the api group of the Role being referenced.
	Group string `json:"group" cue:""`
	// Kind is the type of the Role being referenced.
	Kind string `json:"kind" cue:""`
	// Name is the name of the Role to which this RoleBinding refers.
	Name string `json:"name" cue:""`
}

type Subject struct {
	// Kind is the type of the subject.
	Kind string `json:"kind" cue:""`
	// Name is the name of the subject.
	Name string `json:"name" cue:""`
}
