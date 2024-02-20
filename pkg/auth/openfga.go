package auth

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/openfga/language/pkg/go/transformer"
	openfga "github.com/openfga/openfga/pkg/server"
	"github.com/openfga/openfga/pkg/storage/memory"
)

//go:embed model.fga
var fgaModel string

func (of *OpenFGA) Start(ctx context.Context) error {
	datastore := memory.New()
	ofgaServer, err := openfga.NewServerWithOpts(
		openfga.WithDatastore(datastore),
	)
	ok, err := ofgaServer.IsReady(ctx)
	if err != nil {
		return fmt.Errorf("checking if openfga server is ready: %w", err)
	}
	if !ok {
		return fmt.Errorf("openfga server is not ready")
	}
	of.server = ofgaServer

	createStoreReq := &openfgav1.CreateStoreRequest{Name: "horizon"}
	store, err := ofgaServer.CreateStore(ctx, createStoreReq)
	if err != nil {
		return fmt.Errorf("creating store: %w", err)
	}
	of.storeID = store.Id

	authorizationModel, err := transformer.TransformDSLToProto(fgaModel)
	if err != nil {
		return fmt.Errorf("transforming DSL to proto: %w", err)
	}
	if _, err := ofgaServer.WriteAuthorizationModel(
		ctx,
		&openfgav1.WriteAuthorizationModelRequest{
			StoreId:         store.Id,
			SchemaVersion:   authorizationModel.GetSchemaVersion(),
			TypeDefinitions: authorizationModel.TypeDefinitions,
			Conditions:      authorizationModel.Conditions,
		},
	); err != nil {
		return fmt.Errorf("writing authorization model: %w", err)
	}

	return nil
}

type OpenFGA struct {
	server  *openfga.Server
	storeID string
}

// type CheckRequest struct {
// 	User     string
// 	Relation string
// 	// Object   hz.Objecter
// }

// func (s *Store) Check(ctx context.Context, req CheckRequest) (bool, error) {
// 	resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
// 		StoreId: s.storeID,
// 		TupleKey: &openfgav1.CheckRequestTupleKey{
// 			User:   "user:" + req.User,
// 			Relation: req.Relation,
// 			Object: ,
// 		},
// 	})
// 	if err != nil {
// 		return false, fmt.Errorf("checking authorization: %w", err)
// 	}
// 	return resp.Allowed, nil
// }

type SyncGroupAccountsRequest struct {
	Group    string
	Accounts []GroupAccountRelations
}

type GroupAccountRelations struct {
	Account   string
	Relations []GroupAccountRelation
}

type GroupAccountRelation struct {
	Relation string
	// TODO: Conditions
}

// SyncGroupAccounts updates the relations between a group and its accounts.
// It adds and removes relations as necessary to match the provided state.
// This includes removing relations that are no longer in the provided state.
func (s *OpenFGA) SyncGroupAccounts(
	ctx context.Context,
	req SyncGroupAccountsRequest,
) error {
	readResp, err := s.server.Read(ctx, &openfgav1.ReadRequest{
		StoreId: s.storeID,
		TupleKey: &openfgav1.ReadRequestTupleKey{
			User:   "group:" + req.Group + "#member",
			Object: "account:",
		},
	})
	if err != nil {
		return fmt.Errorf(
			"reading group account: %w",
			err,
		)
	}

	ops := operations{}
	existingAccounts := make(map[string]struct{})
	for _, tuple := range readResp.Tuples {
		accountName := strings.TrimPrefix(tuple.Key.Object, "account:")
		existingAccounts[accountName] = struct{}{}
	}

	// For each account in the provided state, check that its relations already
	// exist, or add them.
	for _, account := range req.Accounts {
		delete(existingAccounts, account.Account)

		accountTuples := filterTuples(
			readResp.Tuples,
			func(t *openfgav1.Tuple) bool {
				return t.Key.Object == "account:"+account.Account
			},
		)
		existingRelations := make(map[string]struct{})
		for _, tuple := range accountTuples {
			existingRelations[tuple.Key.Relation] = struct{}{}
		}
		// For each relation in the provided state, check that it already
		// exists, or add it.
		for _, relation := range account.Relations {
			delete(existingRelations, relation.Relation)

			relTuples := filterTuples(
				accountTuples,
				func(t *openfgav1.Tuple) bool {
					return t.Key.Relation == relation.Relation
				},
			)
			existingAccounts := make(map[string]struct{})
			for _, tuple := range relTuples {
				accountName := strings.TrimPrefix(tuple.Key.Object, "account:")
				existingAccounts[accountName] = struct{}{}
			}
			if _, ok := existingAccounts[account.Account]; ok {
				delete(existingAccounts, account.Account)
			} else {
				ops.add = append(ops.add, &openfgav1.TupleKey{
					User:     "group:" + req.Group + "#member",
					Relation: relation.Relation,
					Object:   "account:" + account.Account,
				})
			}
			for accountName := range existingAccounts {
				ops.delete = append(
					ops.delete,
					&openfgav1.TupleKeyWithoutCondition{
						User:     "group:" + req.Group + "#member",
						Relation: relation.Relation,
						Object:   "account:" + accountName,
					},
				)
			}
		}
		// For each relation that is no longer in the provided state, remove it.
		for relation := range existingRelations {
			ops.delete = append(
				ops.delete,
				&openfgav1.TupleKeyWithoutCondition{
					User:     "group:" + req.Group + "#member",
					Relation: relation,
					Object:   "account:" + account.Account,
				},
			)
		}
	}
	// For each account that is no longer in the provided state, remove it.
	for account := range existingAccounts {
		ops.delete = append(
			ops.delete,
			&openfgav1.TupleKeyWithoutCondition{
				User:   "group:" + req.Group + "#member",
				Object: "account:" + account,
			},
		)
	}

	if len(ops.add) == 0 && len(ops.delete) == 0 {
		// No change, do nothing.
		return nil
	}
	writeReq := openfgav1.WriteRequest{
		StoreId: s.storeID,
	}
	if len(ops.add) > 0 {
		writeReq.Writes = &openfgav1.WriteRequestWrites{
			TupleKeys: ops.add,
		}
	}
	if len(ops.delete) > 0 {
		writeReq.Deletes = &openfgav1.WriteRequestDeletes{
			TupleKeys: ops.delete,
		}
	}
	if _, err := s.server.Write(ctx, &writeReq); err != nil {
		return fmt.Errorf(
			"writing group accounts: %w",
			err,
		)
	}
	return nil
}

type SyncUserGroupMembersRequest struct {
	User   string
	Groups []string
}

func (s *OpenFGA) SyncUserGroupMembers(
	ctx context.Context,
	req SyncUserGroupMembersRequest,
) error {
	readResp, err := s.server.Read(ctx, &openfgav1.ReadRequest{
		StoreId: s.storeID,
		TupleKey: &openfgav1.ReadRequestTupleKey{
			User:     "user:" + req.User,
			Relation: "member",
			Object:   "group:",
		},
	})
	if err != nil {
		return fmt.Errorf(
			"reading user group memberships: %w",
			err,
		)
	}

	// Calculate diff between the groups user is already a part of, and the
	// groups user should be a part of.
	existingGroups := make(map[string]struct{}, len(readResp.Tuples))
	for _, group := range readResp.Tuples {
		groupName := strings.TrimPrefix(group.Key.Object, "group:")
		existingGroups[groupName] = struct{}{}
	}
	ops := operations{}
	for _, group := range req.Groups {
		if _, ok := existingGroups[group]; ok {
			delete(existingGroups, group)
			continue
		}
		ops.add = append(ops.add, &openfgav1.TupleKey{
			User:     "user:" + req.User,
			Relation: "member",
			Object:   "group:" + group,
		})
	}
	for group := range existingGroups {
		ops.delete = append(ops.delete, &openfgav1.TupleKeyWithoutCondition{
			User:     "user:" + req.User,
			Relation: "member",
			Object:   "group:" + group,
		})
	}

	// Apply the calculated operations.
	if len(ops.add) > 0 {
		if _, err := s.server.Write(ctx, &openfgav1.WriteRequest{
			StoreId: s.storeID,
			Writes: &openfgav1.WriteRequestWrites{
				TupleKeys: ops.add,
			},
		}); err != nil {
			return fmt.Errorf(
				"adding user group memberships: %w",
				err,
			)
		}
	}
	if len(ops.delete) > 0 {
		if _, err := s.server.Write(ctx, &openfgav1.WriteRequest{
			StoreId: s.storeID,
			Deletes: &openfgav1.WriteRequestDeletes{
				TupleKeys: ops.delete,
			},
		}); err != nil {
			return fmt.Errorf(
				"deleting user group memberships: %w",
				err,
			)
		}
	}
	return nil
}

// type DeleteUserMembershipsRequest struct {
// 	User string
// }

// func (s *Store) DeleteUserMembership(
// 	ctx context.Context,
// 	req DeleteUserMembershipsRequest,
// ) error {
// 	//
// 	// TODO: REPLACE WITH READ INSTEAD OF LIST
// 	//
// 	listResp, err := s.server.ListObjects(ctx, &openfgav1.ListObjectsRequest{
// 		StoreId:  s.storeID,
// 		User:     "user:" + req.User,
// 		Relation: "member",
// 		Type:     "group",
// 	})
// 	if err != nil {
// 		return fmt.Errorf(
// 			"listing user group memberships: %w",
// 			err,
// 		)
// 	}
// 	deleteOps := make(
// 		[]*openfgav1.TupleKeyWithoutCondition,
// 		len(listResp.Objects),
// 	)

// 	for i, group := range listResp.Objects {
// 		deleteOps[i] = &openfgav1.TupleKeyWithoutCondition{
// 			User:     "user:" + req.User,
// 			Relation: "member",
// 			Object:   group,
// 		}
// 	}
// 	if _, err := s.server.Write(ctx, &openfgav1.WriteRequest{
// 		StoreId: s.storeID,
// 		Deletes: &openfgav1.WriteRequestDeletes{
// 			TupleKeys: deleteOps,
// 		},
// 	}); err != nil {
// 		return fmt.Errorf(
// 			"deleting user group memberships: %w",
// 			err,
// 		)
// 	}
// 	return nil
// }

type operations struct {
	add    []*openfgav1.TupleKey
	delete []*openfgav1.TupleKeyWithoutCondition
}

func filterTuples(
	tuples []*openfgav1.Tuple,
	filter func(*openfgav1.Tuple) bool,
) []*openfgav1.Tuple {
	filtered := make([]*openfgav1.Tuple, 0, len(tuples))
	for _, tuple := range tuples {
		if filter(tuple) {
			filtered = append(filtered, tuple)
		}
	}
	return filtered
}
