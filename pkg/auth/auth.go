package auth

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

func WithInitTimeout(timeout time.Duration) Option {
	return func(o *authorizerOptions) {
		o.timeout = timeout
	}
}

func WithAdminGroups(groups ...string) Option {
	return func(o *authorizerOptions) {
		o.adminGroups = append(o.adminGroups, groups...)
	}
}

type Option func(*authorizerOptions)

type authorizerOptions struct {
	timeout     time.Duration
	adminGroups []string
}

var defaultAuthorizerOptions = authorizerOptions{
	timeout: 5 * time.Second,
}

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
		hz.WithControllerValidatorCUE(),
	)
	if err != nil {
		return fmt.Errorf("starting role controller: %w", err)
	}
	a.controllers = append(a.controllers, ctlrRole)

	ctlrRoleBinding, err := hz.StartController(
		ctx,
		a.Conn,
		hz.WithControllerFor(&RoleBinding{}),
		hz.WithControllerValidatorCUE(),
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
	// if a.Sessions != nil {
	// 	errs = errors.Join(errs, a.Sessions.Close())
	// }
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
	slog.Info("checking", "checkRequest", checkRequest)
	return a.RBAC.Check(ctx, checkRequest), nil
}

// Verb is implied (read).
type ListRequest struct {
	Session string
	Objects []json.RawMessage
}

type ListResponse struct {
	Objects []json.RawMessage
}

func (a *Auth) List(
	ctx context.Context,
	req ListRequest,
) (*ListResponse, error) {
	user, err := a.Sessions.Get(ctx, req.Session)
	if err != nil {
		return nil, err
	}
	resp := ListResponse{
		Objects: []json.RawMessage{},
	}
	for _, rawObj := range req.Objects {
		var obj hz.EmptyObjectWithMeta
		if err := json.Unmarshal(rawObj, &obj); err != nil {
			return nil, fmt.Errorf("unmarshaling object: %w", err)
		}
		ok := a.RBAC.Check(ctx, RBACRequest{
			Groups: user.Groups,
			Verb:   VerbRead,
			Object: obj,
		})
		if ok {
			resp.Objects = append(resp.Objects, rawObj)
		}
	}
	return &resp, nil
}
