package services

import (
	"context"

	"github.com/verifa/horizon/pkg/hz"
)

var _ (hz.Validator) = (*Validator)(nil)

type Validator struct {
	hz.ValidateNothing
}

func (*Validator) ValidateCreate(
	ctx context.Context,
	data []byte,
) error {
	// Implement any custom validation logic here.
	return nil
}
