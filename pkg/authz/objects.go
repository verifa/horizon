package authz

import (
	"context"
	"encoding/base32"
	"fmt"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/verifa/horizon/pkg/hz"
)

type SyncObjectRequest struct {
	Object hz.ObjectKeyer
}

func (s *Store) SyncObject(ctx context.Context, req SyncObjectRequest) error {
	objectID := objecterID(req.Object)
	readResp, err := s.server.Read(ctx, &openfgav1.ReadRequest{
		StoreId: s.storeID,
		TupleKey: &openfgav1.ReadRequestTupleKey{
			User:     "account:" + req.Object.ObjectAccount(),
			Relation: "parent",
			Object:   "object:" + objectID,
		},
	})
	if err != nil {
		return fmt.Errorf("reading object relations: %w", err)
	}
	// If the object relation does not exist, create it.
	if len(readResp.Tuples) == 0 {
		if _, err := s.server.Write(ctx, &openfgav1.WriteRequest{
			StoreId: s.storeID,
			Writes: &openfgav1.WriteRequestWrites{
				TupleKeys: []*openfgav1.TupleKey{
					{
						User:     "account:" + req.Object.ObjectAccount(),
						Relation: "parent",
						Object:   "object:" + objectID,
					},
				},
			},
		}); err != nil {
			return fmt.Errorf("writing object relation: %w", err)
		}
	}

	return nil
}

type DeleteObjectRequest struct {
	Object hz.ObjectKeyer
}

func (s *Store) DeleteObject(
	ctx context.Context,
	req DeleteObjectRequest,
) error {
	objectID := objecterID(req.Object)
	readResp, err := s.server.Read(ctx, &openfgav1.ReadRequest{
		StoreId: s.storeID,
		TupleKey: &openfgav1.ReadRequestTupleKey{
			User:     "account:" + req.Object.ObjectAccount(),
			Relation: "parent",
			Object:   "object:" + objectID,
		},
	})
	if err != nil {
		return fmt.Errorf("reading object relations: %w", err)
	}
	// If the object relation does not exist, do nothing.
	if len(readResp.Tuples) == 0 {
		return nil
	}
	if _, err := s.server.Write(ctx, &openfgav1.WriteRequest{
		StoreId: s.storeID,
		Deletes: &openfgav1.WriteRequestDeletes{
			TupleKeys: []*openfgav1.TupleKeyWithoutCondition{
				{
					User:     "account:" + req.Object.ObjectAccount(),
					Relation: "parent",
					Object:   "object:" + objectID,
				},
			},
		},
	}); err != nil {
		return fmt.Errorf("deleting object relation: %w", err)
	}
	return nil
}

func objecterID(obj hz.ObjectKeyer) string {
	return base32.StdEncoding.EncodeToString([]byte(hz.KeyForObject(obj)))
}
