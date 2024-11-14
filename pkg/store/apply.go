package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"

	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/managedfields"
)

type ApplyRequest struct {
	// Data is the actual JSON payload.
	Data []byte
	// Manager is the name of the field manager for this request.
	Manager string
	// Force will force the apply to happen even if there are conflicts.
	Force    bool
	Key      hz.ObjectKeyer
	IsCreate bool
}

// Apply performs the apply operation on the given request.
// It returns an HTTP status code (int) indicating the result of the operation.
// If an error is returned, the HTTP status code is part of the error and hence
// the return value is ignored (use -1 for consistency).
func (s *Store) Apply(ctx context.Context, req ApplyRequest) (int, error) {
	// For apply, do not validate the request data straight away.
	// An apply might be a patch on an existing object, and as such,
	// validate the end result.
	// If apply is a create, it will get validated.
	// If apply is a patch, validate the merged result.

	// Create managed fields for the request data.
	fieldsV1, err := managedfields.ManagedFieldsV1(req.Data)
	if err != nil {
		return -1, &hz.Error{
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
			return -1, err
		}
		var generic hz.GenericObject
		if err := json.Unmarshal(req.Data, &generic); err != nil {
			return -1, &hz.Error{
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
			return -1, &hz.Error{
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
			return -1, err
		}
		return http.StatusCreated, nil
	}

	// If the object already exists we need to perform a merge of the objects
	// and managed fields.
	// Check if this is a create request. If it is and the object already
	// exists, return a conflict error.
	if req.IsCreate {
		return -1, &hz.Error{
			Status: http.StatusConflict,
			Message: fmt.Sprintf(
				"object already exists: %q",
				req.Key,
			),
		}
	}
	// Decode the existing object's managed fields.
	var generic hz.GenericObject
	if err := json.Unmarshal(rawObj, &generic); err != nil {
		return -1, &hz.Error{
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
			return -1, &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"merging managed fields: %s",
					err.Error(),
				),
			}
		}
		return -1, &hz.Error{
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
		return -1, &hz.Error{
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
		return -1, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"decoding existing object into map[string]interface{}: %s",
				err.Error(),
			),
		}
	}
	if err := json.Unmarshal(req.Data, &src); err != nil {
		return -1, &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"decoding request data into map[string]interface{}: %s",
				err.Error(),
			),
		}
	}
	if err := managedfields.PurgeRemovedFields(dst, result.Removed); err != nil {
		return -1, &hz.Error{
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
		return -1, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"encoding merged object: %s",
				err.Error(),
			),
		}
	}
	// Check if the new object is different from the original object.
	// If there is no change, make it a no-op.
	if isJSONEqual(rawObj, bDst) {
		return http.StatusNotModified, nil
	}
	if err := s.Update(ctx, UpdateRequest{
		Data:     bDst,
		Key:      req.Key,
		Revision: *generic.Revision,
	}); err != nil {
		if errors.Is(err, hz.ErrIncorrectRevision) {
			return -1, &hz.Error{
				Status: http.StatusConflict,
				Message: fmt.Sprintf(
					"updating the object (%s): please try again",
					err.Error(),
				),
			}
		}
		return -1, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"updating object: %s",
				err.Error(),
			),
		}
	}
	return http.StatusOK, nil
}

// isJSONEqual returns true if the JSON objects are "equal".
// Equal means a field by field comparison, not a byte-by-byte comparison.
func isJSONEqual(a, b []byte) bool {
	var aMap, bMap map[string]interface{}
	if err := json.Unmarshal(a, &aMap); err != nil {
		return false
	}
	if err := json.Unmarshal(b, &bMap); err != nil {
		return false
	}
	return reflect.DeepEqual(aMap, bMap)
}
