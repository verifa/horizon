package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type CreateRequest struct {
	Key  string
	Data []byte
}

func (s Store) Create(ctx context.Context, req CreateRequest) error {
	// Check if the object already exists and return a meaningful error.
	if _, err := s.kv.Get(ctx, req.Key); err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"checking existing object: %s",
					err.Error(),
				),
			}
		}
		return s.create(ctx, req.Key, req.Data)
	}
	return &hz.Error{
		Status: http.StatusConflict,
		Message: fmt.Sprintf(
			"object already exists: %q",
			req.Key,
		),
	}
}

func (s Store) create(ctx context.Context, key string, data []byte) error {
	_, err := s.kv.Create(ctx, key, data)
	if err != nil {
		return fmt.Errorf("creating object: %w", err)
	}
	return nil
}
