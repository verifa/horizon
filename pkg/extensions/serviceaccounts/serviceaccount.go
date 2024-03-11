package serviceaccounts

import "github.com/verifa/horizon/pkg/hz"

type ServiceAccount struct {
	hz.ObjectMeta `json:"metadata" cue:""`

	Spec   *ServiceAccountSpec   `json:"spec,omitempty"`
	Status *ServiceAccountStatus `json:"status,omitempty"`
}

func (s ServiceAccount) ObjectGroup() string {
	return "serviceaccounts"
}

func (s ServiceAccount) ObjectVersion() string {
	return "v1"
}

func (s ServiceAccount) ObjectKind() string {
	return "ServiceAccount"
}

type ServiceAccountSpec struct{}

type ServiceAccountStatus struct {
	Ready bool `json:"ready"`

	NATSCredentialsSecretName *string `json:"natsCredentialsSecretName,omitempty"`
}
