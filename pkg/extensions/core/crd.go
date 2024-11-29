package core

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/openapiv3"
)

const (
	ObjectKindCustomResourceDefinition = "CustomResourceDefinition"
)

var (
	_ hz.Objecter               = (*CustomResourceDefinition)(nil)
	_ hz.ObjectOpenAPIV3Schemer = (*CustomResourceDefinition)(nil)
)

type CustomResourceDefinition struct {
	hz.ObjectMeta `json:"metadata,omitempty" cue:""`

	Spec   *CustomResourceDefinitionSpec   `json:"spec,omitempty"`
	Status *CustomResourceDefinitionStatus `json:"status,omitempty"`
}

func (a CustomResourceDefinition) ObjectGroup() string {
	return ObjectGroup
}

func (a CustomResourceDefinition) ObjectVersion() string {
	return ObjectVersion
}

func (a CustomResourceDefinition) ObjectKind() string {
	return "CustomResourceDefinition"
}

func (a CustomResourceDefinition) OpenAPIV3Schema() (*openapiv3.Schema, error) {
	return &openapiv3.Schema{}, nil
}

type CustomResourceDefinitionSpec struct {
	Group   *string                         `json:"group,omitempty" cue:""`
	Version *string                         `json:"version,omitempty" cue:""`
	Names   *CustomResourceDefinitionNames  `json:"names,omitempty" cue:""`
	Schema  *CustomResourceDefinitionSchema `json:"schema,omitempty" cue:""`
}

type CustomResourceDefinitionSchema struct {
	OpenAPIV3Schema *openapiv3.Schema `json:"openAPIV3Schema,omitempty" cue:""`
}

type CustomResourceDefinitionNames struct {
	// Kind is the singular type in PascalCase.
	// Your resource manifests use this.
	Kind *string `json:"kind,omitempty" cue:""`
	// Singular is a lower case alias for the Kind.
	Singular *string `json:"singular,omitempty" cue:""`
}

type CustomResourceDefinitionStatus struct{}

var _ hz.Validator = (*CustomResourceDefinitionValidate)(nil)

type CustomResourceDefinitionValidate struct{}

func (c *CustomResourceDefinitionValidate) ValidateCreate(
	ctx context.Context,
	data []byte,
) error {
	return c.validateName(data)
}

func (c *CustomResourceDefinitionValidate) ValidateUpdate(
	ctx context.Context,
	old []byte,
	data []byte,
) error {
	return c.validateName(data)
}

func (c *CustomResourceDefinitionValidate) validateName(
	data []byte,
) error {
	var crd CustomResourceDefinition
	if err := json.Unmarshal(data, &crd); err != nil {
		return fmt.Errorf(
			"unmarshalling custom resource definition: %w",
			err,
		)
	}
	if crd.Spec.Group == nil {
		return fmt.Errorf("missing group")
	}
	if crd.Spec.Names == nil {
		return fmt.Errorf("missing names")
	}
	if crd.Spec.Names.Kind == nil {
		return fmt.Errorf("missing kind")
	}
	if crd.Spec.Names.Singular == nil {
		return fmt.Errorf("missing singular")
	}
	if crd.Spec.Schema == nil {
		return fmt.Errorf("missing schema")
	}
	if crd.Spec.Schema.OpenAPIV3Schema == nil {
		return fmt.Errorf("missing openAPIV3Schema")
	}

	expectedName := fmt.Sprintf(
		"%s.%s.%s",
		*crd.Spec.Group,
		*crd.Spec.Version,
		*crd.Spec.Names.Kind,
	)
	if crd.ObjectMeta.Name != expectedName {
		return fmt.Errorf(
			"invalid name: %q, expected: %q",
			crd.ObjectMeta.Name,
			expectedName,
		)
	}
	return nil
}
