package auth

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

func WithAdminGroups(groups ...string) Option {
	return func(o *authorizerOptions) {
		o.adminGroups = append(o.adminGroups, groups...)
	}
}

type Option func(*authorizerOptions)

type authorizerOptions struct {
	adminGroups []string
}

var defaultAuthorizerOptions = authorizerOptions{}

func Start(
	ctx context.Context,
	conn *nats.Conn,
	opts ...Option,
) (*Auth, error) {
	auth := Auth{
		Conn: conn,
	}
	err := auth.Start(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("starting auth: %w", err)
	}
	return &auth, nil
}

type Auth struct {
	Conn *nats.Conn

	Sessions *Sessions
	RBAC     *RBAC

	controllers []*hz.Controller
}

func (a *Auth) Start(
	ctx context.Context,
	opts ...Option,
) error {
	ao := defaultAuthorizerOptions
	for _, opt := range opts {
		opt(&ao)
	}

	//
	// Start controllers.
	//
	ctlrRole, err := hz.StartController(
		ctx,
		a.Conn,
		hz.WithControllerFor(&Role{}),
	)
	if err != nil {
		return fmt.Errorf("starting role controller: %w", err)
	}
	a.controllers = append(a.controllers, ctlrRole)

	ctlrRoleBinding, err := hz.StartController(
		ctx,
		a.Conn,
		hz.WithControllerFor(&RoleBinding{}),
	)
	if err != nil {
		return fmt.Errorf("starting rolebinding controller: %w", err)
	}
	a.controllers = append(a.controllers, ctlrRoleBinding)

	//
	// Start the session manager.
	//
	sessions := Sessions{
		Conn: a.Conn,
	}
	if err := sessions.Start(ctx); err != nil {
		return fmt.Errorf("starting session manager: %w", err)
	}
	a.Sessions = &sessions

	rbac := RBAC{
		Conn:         a.Conn,
		RoleBindings: make(map[string]RoleBinding),
		Roles:        make(map[string]Role),
		Permissions:  make(map[string]*Group),
		AdminGroups:  ao.adminGroups,
	}
	if err := rbac.Start(ctx); err != nil {
		return fmt.Errorf("starting rbac: %w", err)
	}
	a.RBAC = &rbac
	return nil
}

func (a *Auth) Close() error {
	var errs error
	if a.RBAC != nil {
		errs = errors.Join(errs, a.RBAC.Close())
	}
	for _, c := range a.controllers {
		errs = errors.Join(errs, c.Stop())
	}
	return errs
}

type CheckRequest struct {
	Session string
	Verb    Verb
	Object  hz.ObjectKeyer
}

func (a *Auth) Check(
	ctx context.Context,
	req CheckRequest,
) (bool, error) {
	user, err := a.Sessions.Get(ctx, req.Session)
	if err != nil {
		return false, err
	}
	checkRequest := RBACRequest{
		Groups: user.Groups,
		Verb:   req.Verb,
		Object: req.Object,
	}
	ok := a.RBAC.Check(ctx, checkRequest)
	slog.Info("checking", "checkRequest", checkRequest, "ok", ok)
	return ok, nil
}

// Verb is implied (read).
type ListRequest struct {
	Session    string
	ObjectList *hz.ObjectList
}

func (a *Auth) List(
	ctx context.Context,
	req ListRequest,
) error {
	user, err := a.Sessions.Get(ctx, req.Session)
	if err != nil {
		return err
	}
	filteredObjects := make([]json.RawMessage, 0)
	for _, rawObj := range req.ObjectList.Items {
		var obj hz.MetaOnlyObject
		if err := json.Unmarshal(rawObj, &obj); err != nil {
			return fmt.Errorf("unmarshaling object: %w", err)
		}
		ok := a.RBAC.Check(ctx, RBACRequest{
			Groups: user.Groups,
			Verb:   VerbRead,
			Object: obj,
		})
		if ok {
			filteredObjects = append(filteredObjects, rawObj)
		}
	}
	req.ObjectList.Items = filteredObjects
	return nil
}
