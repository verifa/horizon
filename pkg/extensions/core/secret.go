package core

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Secret)(nil)

type Secret struct {
	hz.ObjectMeta `           json:"metadata" cue:""`
	Data          SecretData `json:"data"     cue:",optional"`
}

func (s Secret) ObjectGroup() string {
	return ObjectGroup
}

func (s Secret) ObjectVersion() string {
	return ObjectVersion
}

func (s Secret) ObjectKind() string {
	return "Secret"
}

type SecretData map[string]string
