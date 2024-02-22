package auth

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Role)(nil)

type Role struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec RoleSpec `json:"spec,omitempty" cue:""`
}

func (Role) ObjectAPIVersion() string {
	return "v1"
}

func (Role) ObjectGroup() string {
	return "hz-internal"
}

func (Role) ObjectKind() string {
	return "Role"
}

type RoleSpec struct {
	Allow []Verbs `json:"allow,omitempty"`
	Deny  []Verbs `json:"deny,omitempty"`
}

type Verbs struct {
	Read   *VerbFilter `json:"read,omitempty"`
	Update *VerbFilter `json:"update,omitempty"`
	Create *VerbFilter `json:"create,omitempty"`
	Delete *VerbFilter `json:"delete,omitempty"`
	Run    *VerbFilter `json:"run,omitempty"`
}

type VerbFilter struct {
	Name  *string `json:"name,omitempty" cue:""`
	Kind  *string `json:"kind,omitempty" cue:""`
	Group *string `json:"group,omitempty" cue:""`
}
