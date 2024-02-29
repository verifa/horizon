package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

type ListRequest struct {
	Key hz.ObjectKeyer `json:"key,omitempty"`
}

// type ListResponse struct {
// 	Data []json.RawMessage `json:"data"`
// }

func (s Store) List(
	ctx context.Context,
	req ListRequest,
) (*hz.ObjectList, error) {
	wOpts := []jetstream.WatchOpt{jetstream.IgnoreDeletes()}
	watcher, err := s.kv.Watch(ctx, hz.KeyFromObject(req.Key), wOpts...)
	if err != nil {
		return nil, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("watching key: %s", err.Error()),
		}
	}
	defer func() {
		_ = watcher.Stop()
	}()

	objects := []json.RawMessage{}
	for entry := range watcher.Updates() {
		if entry == nil {
			break
		}
		data, err := s.toObjectWithRevision(entry)
		if err != nil {
			return nil, fmt.Errorf("formatting data: %w", err)
		}
		objects = append(objects, data)
	}
	return &hz.ObjectList{
		Items: objects,
	}, nil
}
