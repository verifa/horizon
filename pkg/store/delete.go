package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/hz"
)

type DeleteRequest struct {
	Key hz.ObjectKeyer
}

func (s *Store) Delete(ctx context.Context, req DeleteRequest) error {
	data, err := s.get(ctx, req.Key)
	if err != nil {
		return err
	}
	var obj hz.MetaOnlyObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("unmarshalling object: %s", err.Error()),
		}
	}
	if obj.ObjectMeta.Revision == nil {
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "object revision is nil",
		}
	}
	revision := *obj.ObjectMeta.Revision
	deleteAt := hz.Time{Time: time.Now()}
	data, err = sjson.SetBytes(data, "metadata.deletionTimestamp", deleteAt)
	if err != nil {
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("setting deletion timestamp: %s", err.Error()),
		}
	}
	if err := s.Update(ctx, UpdateRequest{
		Data:     data,
		Key:      req.Key,
		Revision: revision,
	}); err != nil {
		return err
	}
	return nil
}
