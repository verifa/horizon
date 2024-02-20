package auth

import (
	"context"
	"testing"

	openfgav1 "github.com/openfga/api/proto/openfga/v1"
	"github.com/verifa/horizon/pkg/hz"
	tu "github.com/verifa/horizon/pkg/testutil"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestOpenFGA(t *testing.T) {
	ctx := context.Background()
	s := OpenFGA{}
	err := s.Start(ctx)
	tu.AssertNoError(t, err)

	userID := "testuser"
	userGroups := []string{"test"}

	err = s.SyncUserGroupMembers(ctx, SyncUserGroupMembersRequest{
		User:   userID,
		Groups: userGroups,
	})
	tu.AssertNoError(t, err)

	err = s.SyncGroupAccounts(ctx, SyncGroupAccountsRequest{
		Group: "test",
		Accounts: []GroupAccountRelations{
			{
				Account: "test",
				Relations: []GroupAccountRelation{
					{
						Relation: "viewer",
					},
					{
						Relation: "editor",
					},
				},
			},
		},
	})
	tu.AssertNoError(t, err)

	// TODO: use store method not server directly...
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "member",
				Object:   "group:test",
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, true, resp.Allowed)
	}
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "can_read",
				Object:   "account:test",
			},
			Context: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"object": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"kind": structpb.NewStringValue("test"),
						},
					}),
				},
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, true, resp.Allowed)
	}
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "can_update",
				Object:   "account:test",
			},
			Context: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"object": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"kind": structpb.NewStringValue("test"),
						},
					}),
				},
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, true, resp.Allowed)
	}

	err = s.SyncGroupAccounts(ctx, SyncGroupAccountsRequest{
		Group: "test",
		Accounts: []GroupAccountRelations{
			{
				Account: "test",
				Relations: []GroupAccountRelation{
					{
						Relation: "viewer",
					},
					// {
					// 	Relation: "editor",
					// },
				},
			},
		},
	})
	tu.AssertNoError(t, err)
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "can_create",
				Object:   "account:test",
			},
			Context: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"object": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"kind": structpb.NewStringValue("test"),
						},
					}),
				},
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, false, resp.Allowed)
	}

	{
		err := s.SyncObject(ctx, SyncObjectRequest{
			Object: hz.Key{
				Name:    "test",
				Account: "test",
				Kind:    "Test",
			},
		})
		tu.AssertNoError(t, err)
	}
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "can_read",
				Object: "object:" + objecterID(hz.Key{
					Name:    "test",
					Account: "test",
					Kind:    "Test",
				}),
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, true, resp.Allowed)
	}
	{
		err := s.DeleteObject(ctx, DeleteObjectRequest{
			Object: hz.Key{
				Name:    "test",
				Account: "test",
				Kind:    "Test",
			},
		})
		tu.AssertNoError(t, err)
	}
	{
		resp, err := s.server.Check(ctx, &openfgav1.CheckRequest{
			StoreId: s.storeID,
			TupleKey: &openfgav1.CheckRequestTupleKey{
				User:     "user:" + userID,
				Relation: "can_read",
				Object: "object:" + objecterID(hz.Key{
					Name:    "test",
					Account: "test",
					Kind:    "Test",
				}),
			},
		})
		tu.AssertNoError(t, err)
		tu.AssertEqual(t, false, resp.Allowed)
	}
}
