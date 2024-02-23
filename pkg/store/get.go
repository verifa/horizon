package store

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/hz"
)

type GetRequest struct {
	Key hz.ObjectKey
}

func (s Store) Get(ctx context.Context, req GetRequest) ([]byte, error) {
	data, err := s.get(ctx, req.Key)
	if err != nil {
		return nil, err
	}
	return data, err
}

func (s Store) get(ctx context.Context, key hz.ObjectKey) ([]byte, error) {
	kve, err := s.kv.Get(ctx, hz.KeyFromObject(key))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, hz.ErrNotFound
		}
	}
	return s.toObjectWithRevision(kve)
}

// toObjectWithRevision takes a KeyValueEntry and adds the revision to the
// metadata of the JSON bytes.
// This is quite a horrible and hacky approach that should probably be fixed in
// the future, but it works for now and keeps the interfaces clean.
func (s Store) toObjectWithRevision(
	kve jetstream.KeyValueEntry,
) ([]byte, error) {
	data, err := sjson.SetBytes(
		kve.Value(),
		"metadata.revision",
		kve.Revision(),
	)
	if err != nil {
		return nil, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"setting revision: %s",
				err.Error(),
			),
		}
	}
	return data, nil
}
