package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type UpdateRequest struct{}

func (s Store) Update(ctx context.Context, req UpdateRequest) error {
	return errors.New("TODO")
}

func (s Store) update(
	ctx context.Context,
	key hz.ObjectKey,
	data []byte,
	revision uint64,
) (uint64, error) {
	rawKey, err := hz.KeyFromObjectConcrete(key)
	if err != nil {
		return 0, &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"invalid key: %q",
				err.Error(),
			),
		}
	}
	newRevision, err := s.kv.Update(ctx, rawKey, data, revision)
	if err != nil {
		if isErrWrongLastSequence(err) {
			return 0, hz.ErrIncorrectRevision
		}
		return 0, fmt.Errorf("update: %w", err)
	}
	return newRevision, nil
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
