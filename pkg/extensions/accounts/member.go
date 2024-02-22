package accounts

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Member)(nil)

type Member struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec MemberSpec `json:"spec,omitempty" cue:""`
}

func (Member) ObjectKind() string {
	return "Member"
}

func (Member) ObjectGroup() string {
	return "hz-internal"
}

type MemberSpec struct {
	GroupRef *GroupRef `json:"groupRef,omitempty" cue:""`
	UserRef  *UserRef  `json:"userRef,omitempty" cue:""`
}

type GroupRef struct {
	Name *string `json:"name,omitempty" cue:""`
}

type UserRef struct {
	Name *string `json:"name,omitempty" cue:""`
}
