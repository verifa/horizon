package auth

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Role)(nil)

type Role struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec RoleSpec `json:"spec,omitempty" cue:""`
}

func (Role) ObjectVersion() string {
	return "v1"
}

func (Role) ObjectGroup() string {
	return "core"
}

func (Role) ObjectKind() string {
	return "Role"
}

type RoleSpec struct {
	Allow []Rule `json:"allow,omitempty"`
	Deny  []Rule `json:"deny,omitempty"`
}

type Rule struct {
	// Name of a resource that this rule targets.
	Name *string `json:"name,omitempty" cue:""`
	// Kind of a resource that this rule targets.
	Kind *string `json:"kind,omitempty" cue:""`
	// Group of a resource that this rule targets.
	Group *string `json:"group,omitempty" cue:""`
	// Verbs that this rule enforces.
	Verbs []Verb `json:"verbs,omitempty" cue:""`
}
