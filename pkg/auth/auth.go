package auth

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/extensions/accounts"
	"github.com/verifa/horizon/pkg/hz"
)

func WithInitTimeout(timeout time.Duration) AuthorizerOption {
	return func(o *authorizerOptions) {
		o.timeout = timeout
	}
}

type AuthorizerOption func(*authorizerOptions)

type authorizerOptions struct {
	timeout time.Duration
}

var defaultAuthorizerOptions = authorizerOptions{
	timeout: 5 * time.Second,
}

func Start(ctx context.Context, conn *nats.Conn) (*Auth, error) {
	auth := Auth{
		Conn: conn,
	}
	err := auth.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("starting auth: %w", err)
	}
	return &auth, nil
}

type Auth struct {
	Conn *nats.Conn

	Sessions   *Sessions
	Authorizer *OpenFGA

	watchers []*hz.Watcher
}

func (a *Auth) Start(
	ctx context.Context,
	opts ...AuthorizerOption,
) error {
	ao := defaultAuthorizerOptions
	for _, opt := range opts {
		opt(&ao)
	}

	//
	// Start the session manager.
	//
	sessions := Sessions{
		Conn: a.Conn,
	}
	err := sessions.Start(ctx)
	if err != nil {
		return fmt.Errorf("starting session manager: %w", err)
	}
	a.Sessions = &sessions

	//
	// Start the authorizer.
	//
	authz := OpenFGA{}
	err = authz.Start(ctx)
	if err != nil {
		return fmt.Errorf("creating openfga store: %w", err)
	}
	a.Authorizer = &authz

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

	// Wait for all watchers to initialize.
	init := make(chan struct{})
	go func() {
		for _, w := range a.watchers {
			<-w.Init
		}
		close(init)
	}()

	select {
	case <-init:
		// Do nothing and continue.
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timed out waiting for watchers to initialize")
	}

	return nil
}

func (a *Auth) Close() {
	for _, w := range a.watchers {
		w.Close()
	}
}

// type CheckRequest struct {
// 	User      string
// 	Operation string
// 	Target
// }

// func (a *Authorizer) handleCheckRequest(
// 	ctx context.Context,
// 	req CheckRequest,
// ) (bool, error) {
// 	a.store.server.Check(ctx, &openfgav1.CheckRequest{
// 		StoreId: a.store.storeID,
// 		TupleKey: &openfgav1.CheckRequestTupleKey{
// 			User: "user:" + req.User,
// 		},
// 	})
// }

func (a *Auth) handleUserEvent(
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
		err := a.Authorizer.SyncUserGroupMembers(
			ctx,
			SyncUserGroupMembersRequest{
				User:   userID,
				Groups: user.Spec.Claims.Groups,
			},
		)
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"updating user group memberships: %w",
				err,
			)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		err := a.Authorizer.SyncUserGroupMembers(
			ctx,
			SyncUserGroupMembersRequest{
				User: userID,
				// Empty groups to remove all user-->group relations.
				Groups: []string{},
			},
		)
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

func (a *Auth) handleGroupEvent(
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
		err := a.Authorizer.SyncGroupAccounts(ctx, req)
		if err != nil {
			return hz.Result{}, fmt.Errorf(
				"updating group accounts: %w",
				err,
			)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		err := a.Authorizer.SyncGroupAccounts(ctx, SyncGroupAccountsRequest{
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

func (a *Auth) handleObjectEvent(
	ctx context.Context,
	event hz.Event,
) (hz.Result, error) {
	key, err := hz.KeyFromString(event.Key)
	if err != nil {
		return hz.Result{}, fmt.Errorf("parsing group key: %w", err)
	}
	switch event.Operation {
	case hz.EventOperationPut:
		if err := a.Authorizer.SyncObject(ctx, SyncObjectRequest{
			Object: key,
		}); err != nil {
			return hz.Result{}, fmt.Errorf("syncing object: %w", err)
		}
	case hz.EventOperationDelete, hz.EventOperationPurge:
		if err := a.Authorizer.DeleteObject(ctx, DeleteObjectRequest{
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
