package accounts

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Group)(nil)

type Group struct {
	hz.ObjectMeta `json:"metadata,omitempty"`

	Spec GroupSpec `json:"spec,omitempty" cue:""`
}

func (Group) ObjectKind() string {
	return "Group"
}

type GroupSpec struct {
	Accounts map[string]GroupAccount `json:"accounts,omitempty"`
}

type GroupAccount struct {
	Relations map[string]GroupAccountRelation `json:"relations,omitempty"`
}

// GroupAccountRelation represents a relation between a group and an account.
// This could contain conditions in the future.
type GroupAccountRelation struct{}
