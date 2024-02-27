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
	Key  hz.ObjectKey
	Data []byte
}

func (s Store) Create(ctx context.Context, req CreateRequest) error {
	// Check if the object already exists and return a meaningful error.
	if _, err := s.kv.Get(ctx, hz.KeyFromObject(req.Key)); err != nil {
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"checking existing object: %s",
					err.Error(),
				),
			}
		}
		if err := s.validate(ctx, req.Key, req.Data); err != nil {
			return &hz.Error{
				Status: http.StatusBadRequest,
				Message: fmt.Sprintf(
					"validating object %q: %s",
					req.Key,
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

func (s Store) create(
	ctx context.Context,
	key hz.ObjectKey,
	data []byte,
) error {
	_, err := s.kv.Create(ctx, hz.KeyFromObject(key), data)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return &hz.Error{
				Status: http.StatusConflict,
				Message: fmt.Sprintf(
					"object already exists: %q",
					key,
				),
			}
		}
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"creating object: %s",
				err.Error(),
			),
		}
	}
	return nil
}
