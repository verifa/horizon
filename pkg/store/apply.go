package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/hz"
)

type ApplyRequest struct {
	// Data is the actual JSON payload.
	Data []byte
	// Manager is the name of the field manager for this request.
	Manager string
	Kind    string
	Key     string
}

func (s Store) Apply(ctx context.Context, req ApplyRequest) error {
	slog.Info("apply", "req", req)
	if err := s.validate(ctx, req.Kind, req.Data); err != nil {
		return &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("validating object: %s", err.Error()),
		}
	}

	fieldsV1, err := ManagedFieldsV1(req.Data)
	if err != nil {
		return &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"creating field manager: %s",
				err.Error(),
			),
		}
	}
	fieldManager := hz.FieldManager{
		Manager:    req.Manager,
		FieldsV1:   fieldsV1,
		FieldsType: "FieldsV1",
	}

	rawObj, err := s.get(ctx, req.Key)
	if err != nil {
		if !errors.Is(err, hz.ErrNotFound) {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"checking existing object: %s",
					err.Error(),
				),
			}
		}
		// Object does not exist.
		// In this case, create the object from the payload.
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
		b, err := json.Marshal([]hz.FieldManager{fieldManager})
		if err != nil {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"marshalling field manager: %s",
					err.Error(),
				),
			}
		}
		// Set the kind as this might not exist in the JSON payload.
		generic.Kind = req.Kind
		generic.ManagedFields = b
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
		if err := s.create(ctx, req.Key, bGeneric); err != nil {
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

	// If the object already exists we need to perform a merge.
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
	var fieldManagers []hz.FieldManager
	// If the stored JSON does not contain any managed fields, skip decoding.
	// Otherwise we get an error because we are trying to decode an empty JSON
	// string (which is invalid JSON).
	if len(generic.ManagedFields) != 0 {
		if err := json.Unmarshal(generic.ManagedFields, &fieldManagers); err != nil {
			return &hz.Error{
				Status: http.StatusInternalServerError,
				Message: fmt.Sprintf(
					"decoding existing managed fields: %s",
					err.Error(),
				),
			}
		}
	}
	result, err := MergeManagedFields(fieldManagers, fieldManager)
	if err != nil {
		var conflictErr *Conflict
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

	newObj, err := sjson.SetBytes(
		rawObj,
		"metadata.managedFields",
		result.ManagedFields,
	)
	if err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"setting managed fields: %s",
				err.Error(),
			),
		}
	}

	// Create map[string]interface{} values for the existing object (dst) and
	// the request object (src).
	// Then purge any removed fields (if any) from the manager in src, in dst.
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
	if err := PurgeRemovedFields(dst, result.Removed); err != nil {
		return &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"purging removed fields: %s",
				err.Error(),
			),
		}
	}
	MergeObjects(dst, src)
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
	if _, err := s.update(ctx, req.Key, bDst, *generic.Revision); err != nil {
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
