package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type UpdateRequest struct{}

func (s Store) Update(ctx context.Context, req UpdateRequest) error {
	return errors.New("TODO")
}

func (s Store) update(
	ctx context.Context,
	key string,
	data []byte,
	revision uint64,
) (uint64, error) {
	revision, err := s.kv.Update(ctx, key, data, revision)
	if err != nil {
		if isErrWrongLastSequence(err) {
			return 0, hz.ErrIncorrectRevision
		}
		return 0, fmt.Errorf("update: %w", err)
	}
	return revision, nil
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
