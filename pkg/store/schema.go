package store

import (
	"context"
	"errors"
)

type SchemaRequest struct{}

func (s *Store) Schema(ctx context.Context, req SchemaRequest) error {
	return errors.New("TODO")
}
