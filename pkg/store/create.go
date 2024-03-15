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
	Key  hz.ObjectKeyer
	Data []byte
}

func (s *Store) Create(ctx context.Context, req CreateRequest) error {
	// Check if the object already exists and return a meaningful error.
	if _, err := s.kv.Get(ctx, hz.KeyFromObject(req.Key)); err != nil {
		// If we get a non ErrKeyNotFound error, something went wrong...
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"checking existing object: %s",
					err.Error(),
				),
			}
		}
		if err := s.validateCreate(ctx, req.Key, req.Data); err != nil {
			return hz.ErrorWrap(
				err,
				http.StatusInternalServerError,
				fmt.Sprintf("validating object: %q", req.Key),
			)
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

func (s *Store) create(
	ctx context.Context,
	key hz.ObjectKeyer,
	data []byte,
) error {
	rawKey, err := hz.KeyFromObjectStrict(key)
	if err != nil {
		return &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"invalid key: %q",
				err.Error(),
			),
		}
	}
	data, err = removeReadOnlyFields(data)
	if err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"removing read-only fields: %s",
				err.Error(),
			),
		}
	}
	if _, err := s.kv.Create(ctx, rawKey, data); err != nil {
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
