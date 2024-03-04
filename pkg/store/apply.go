package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/managedfields"
)

type ApplyRequest struct {
	// Data is the actual JSON payload.
	Data []byte
	// Manager is the name of the field manager for this request.
	Manager string
	// Force will force the apply to happen even if there are conflicts.
	Force bool
	Key   hz.ObjectKeyer
}

func (s Store) Apply(ctx context.Context, req ApplyRequest) error {
	// For apply, do not validate the request data straight away.
	// An apply might be a patch on an existing object, and as such,
	// validate the end result.
	// If apply is a create, it will get validated.
	// If apply is a patch, validate the merged result.

	// Create managed fields for the request data.
	fieldsV1, err := managedfields.ManagedFieldsV1(req.Data)
	if err != nil {
		return &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"creating field manager: %s",
				err.Error(),
			),
		}
	}
	fieldManager := managedfields.FieldManager{
		Manager:    req.Manager,
		FieldsV1:   fieldsV1,
		FieldsType: managedfields.FieldsTypeV1,
	}

	// Get the existing object (if it exists).
	// If not, add the managed fields to the request and create the object.
	rawObj, err := s.get(ctx, req.Key)
	if err != nil {
		if !errors.Is(err, hz.ErrNotFound) {
			return err
		}
		var generic hz.GenericObject
		if err := json.Unmarshal(req.Data, &generic); err != nil {
			return &hz.Error{
				Status: http.StatusBadRequest,
				Message: fmt.Sprintf(
					"decoding request data: %s",
					err.Error(),
				),
			}
		}
		generic.ManagedFields = []managedfields.FieldManager{fieldManager}
		bGeneric, err := json.Marshal(generic)
		if err != nil {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"marshalling generic object: %s",
					err.Error(),
				),
			}
		}
		// Create and validate the object.
		if err := s.Create(ctx, CreateRequest{
			Key:  req.Key,
			Data: bGeneric,
		}); err != nil {
			return err
		}
		return nil
	}

	// If the object already exists we need to perform a merge of the objects
	// and managed fields.
	// Decode the existing object's managed fields.
	var generic hz.GenericObject
	if err := json.Unmarshal(rawObj, &generic); err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"decoding existing object: %s",
				err.Error(),
			),
		}
	}
	// Merge managed fields and detect any conflicts.
	result, err := managedfields.MergeManagedFields(
		generic.ManagedFields,
		fieldManager,
		req.Force,
	)
	if err != nil {
		var conflictErr *managedfields.Conflict
		if !errors.As(err, &conflictErr) {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"merging managed fields: %s",
					err.Error(),
				),
			}
		}
		return &hz.Error{
			Status: http.StatusConflict,
			Message: fmt.Sprintf(
				"conflict: %s",
				err.Error(),
			),
		}
	}

	generic.ManagedFields = result.ManagedFields
	newObj, err := json.Marshal(generic)
	if err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"marshalling merged managed fields object: %s",
				err.Error(),
			),
		}
	}

	// Create map[string]interface{} values for the existing object (dst) and
	// the request object (src).
	// Then purge any removed fields (if any) from dst.
	// Finally merge src into dst.
	var dst, src map[string]interface{}
	if err := json.Unmarshal(newObj, &dst); err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"decoding existing object into map[string]interface{}: %s",
				err.Error(),
			),
		}
	}
	if err := json.Unmarshal(req.Data, &src); err != nil {
		return &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"decoding request data into map[string]interface{}: %s",
				err.Error(),
			),
		}
	}
	if err := managedfields.PurgeRemovedFields(dst, result.Removed); err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"purging removed fields: %s",
				err.Error(),
			),
		}
	}
	managedfields.MergeObjects(dst, src, fieldsV1)
	bDst, err := json.Marshal(dst)
	if err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"encoding merged object: %s",
				err.Error(),
			),
		}
	}
	if err := s.Update(ctx, UpdateRequest{
		Data:     bDst,
		Key:      req.Key,
		Revision: *generic.Revision,
	}); err != nil {
		if errors.Is(err, hz.ErrIncorrectRevision) {
			return &hz.Error{
				Status: http.StatusConflict,
				Message: fmt.Sprintf(
					"updating the object (%s): please try again",
					err.Error(),
				),
			}
		}
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"updating object: %s",
				err.Error(),
			),
		}
	}
	return nil
}
