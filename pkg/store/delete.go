package store

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/hz"
)

type DeleteRequest struct {
	Key     hz.ObjectKey
	Manager string
}

func (s Store) Delete(ctx context.Context, req DeleteRequest) error {
	data, err := s.get(ctx, req.Key)
	if err != nil {
		return err
	}
	deleteAt := hz.Time{Time: time.Now()}
	data, err = sjson.SetBytes(data, "metadata.deletionTimestamp", deleteAt)
	if err != nil {
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("setting deletion timestamp: %s", err.Error()),
		}
	}
	if err := s.Apply(ctx, ApplyRequest{
		Data:    data,
		Manager: req.Manager,
	}); err != nil {
		return err
	}
	return nil
}
