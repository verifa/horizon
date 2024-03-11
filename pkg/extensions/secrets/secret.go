package secrets

import "github.com/verifa/horizon/pkg/hz"

var _ hz.Objecter = (*Secret)(nil)

type Secret struct {
	hz.ObjectMeta `json:"metadata" cue:""`
	Data          SecretData `json:"data" cue:",optional"`
}

func (s Secret) ObjectGroup() string {
	return "secrets"
}

func (s Secret) ObjectVersion() string {
	return "v1"
}

func (s Secret) ObjectKind() string {
	return "Secret"
}

type SecretData map[string]string
