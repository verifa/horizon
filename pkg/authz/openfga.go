package authz

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
)

type Authorizer struct {
	Conn *nats.Conn

	store *Store

	watchers []*hz.Watcher
}

func (a *Authorizer) Start(ctx context.Context) error {
	store, err := NewStore(ctx)
	if err != nil {
		return fmt.Errorf("creating openfga store: %w", err)
	}
	a.store = store
	//
	// Start user watcher
	//
	userWatcher, err := hz.StartWatcher(
		ctx,
		a.Conn,
		hz.WithWatcherForObject(accounts.User{}),
		hz.WithWatcherFn(a.handleUserEvent),
	)
	if err != nil {
		return fmt.Errorf("starting user watcher: %w", err)
	}
	a.watchers = append(a.watchers, userWatcher)
	//
	// Start group watcher
	//
	groupWatcher, err := hz.StartWatcher(
		ctx,
		a.Conn,
		hz.WithWatcherForObject(accounts.Group{}),
		hz.WithWatcherFn(a.handleGroupEvent),
	)
	if err != nil {
		return fmt.Errorf("starting group watcher: %w", err)
	}
	a.watchers = append(a.watchers, groupWatcher)
	//
	// Start object watcher
	//
	objectWatcher, err := hz.StartWatcher(
		ctx,
		a.Conn,
		hz.WithWatcherForObject(hz.KeyAllObjects),
		hz.WithWatcherFn(a.handleObjectEvent),
	)
	if err != nil {
		return fmt.Errorf("starting object watcher: %w", err)
	}
	a.watchers = append(a.watchers, objectWatcher)

	return nil
}

func (a *Authorizer) Close() {
	for _, w := range a.watchers {
		w.Close()
	}
}

func (a *Authorizer) handleUserEvent(
	ctx context.Context,
	event hz.Event,
) (hz.Result, error) {
	key, err := hz.KeyFromString(event.Key)
	if err != nil {
		return hz.Result{}, fmt.Errorf("parsing group key: %w", err)
	}
	userID := key.Name

	switch event.Operation {
	case hz.EventOperationPut:
		var user accounts.User
		if err := json.Unmarshal(event.Data, &user); err != nil {
			return hz.Result{}, fmt.Errorf("unmarshalling user: %w", err)
		}
		err := a.store.SyncUserGroupMembers(ctx, SyncUserGroupMembersRequest{
			User:   userID,
			Groups: user.Spec.Claims.Groups,
		})
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"updating user group memberships: %w",
				err,
			)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		err := a.store.SyncUserGroupMembers(ctx, SyncUserGroupMembersRequest{
			User: userID,
			// Empty groups to remove all user-->group relations.
			Groups: []string{},
		})
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"deleting user group memberships: %w",
				err,
			)
		}
	default:
		return hz.Result{}, fmt.Errorf(
			"unexpected event operation: %v",
			event.Operation,
		)
	}
	return hz.Result{}, nil
}

func (a *Authorizer) handleGroupEvent(
	ctx context.Context,
	event hz.Event,
) (hz.Result, error) {
	key, err := hz.KeyFromString(event.Key)
	if err != nil {
		return hz.Result{}, fmt.Errorf("parsing group key: %w", err)
	}
	groupID := key.Name

	switch event.Operation {
	case hz.EventOperationPut:
		var group accounts.Group
		if err := json.Unmarshal(event.Data, &group); err != nil {
			return hz.Result{}, fmt.Errorf("unmarshalling group: %w", err)
		}
		req := groupAccountsToRequest(group)
		err := a.store.SyncGroupAccounts(ctx, req)
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"updating group accounts: %w",
				err,
			)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		err := a.store.SyncGroupAccounts(ctx, SyncGroupAccountsRequest{
			Group: groupID,
			// Empty accounts to remove all group-->account relations.
			Accounts: []GroupAccountRelations{},
		})
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"deleting group accounts: %w",
				err,
			)
		}
	default:
		return hz.Result{}, fmt.Errorf(
			"unexpected event operation: %v",
			event.Operation,
		)
	}

	return hz.Result{}, nil
}

func (a *Authorizer) handleObjectEvent(
	ctx context.Context,
	event hz.Event,
) (hz.Result, error) {
	key, err := hz.KeyFromString(event.Key)
	if err != nil {
		return hz.Result{}, fmt.Errorf("parsing group key: %w", err)
	}
	switch event.Operation {
	case hz.EventOperationPut:
		if err := a.store.SyncObject(ctx, SyncObjectRequest{
			Object: key,
		}); err != nil {
			return hz.Result{}, fmt.Errorf("syncing object: %w", err)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		if err := a.store.DeleteObject(ctx, DeleteObjectRequest{
			Object: key,
		}); err != nil {
			return hz.Result{}, fmt.Errorf("deleting object: %w", err)
		}
	default:
		return hz.Result{}, fmt.Errorf(
			"unexpected event operation: %v",
			event.Operation,
		)
	}

	return hz.Result{}, nil
}

func groupAccountsToRequest(group accounts.Group) SyncGroupAccountsRequest {
	var accounts []GroupAccountRelations
	for account, relations := range group.Spec.Accounts {
		var groupRelations []GroupAccountRelation
		for relation := range relations.Relations {
			groupRelations = append(groupRelations, GroupAccountRelation{
				Relation: relation,
			})
		}
		accounts = append(accounts, GroupAccountRelations{
			Account:   account,
			Relations: groupRelations,
		})
	}
	return SyncGroupAccountsRequest{
		Group:    group.Name,
		Accounts: accounts,
	}
}
