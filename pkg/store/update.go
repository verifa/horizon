package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type UpdateRequest struct {
	Data     []byte
	Key      hz.ObjectKeyer
	Revision uint64
}

func (s Store) Update(ctx context.Context, req UpdateRequest) error {
	if err := s.validate(ctx, req.Key, req.Data); err != nil {
		return hz.ErrorWrap(
			err,
			http.StatusInternalServerError,
			fmt.Sprintf("validating object: %q", req.Key),
		)
	}
	return s.update(ctx, req.Key, req.Data, req.Revision)
}

func (s Store) update(
	ctx context.Context,
	key hz.ObjectKeyer,
	data []byte,
	revision uint64,
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
	if _, err := s.kv.Update(ctx, rawKey, data, revision); err != nil {
		if isErrWrongLastSequence(err) {
			return hz.ErrIncorrectRevision
		}
		return fmt.Errorf("update: %w", err)
	}
	return nil
}

// isErrWrongLastSequence returns true if the error is caused by a write
// operation to a stream with the wrong last sequence.
// For example, if a kv update with an outdated revision.
func isErrWrongLastSequence(err error) bool {
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence
	}
	return false
}
