package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

const (
	bucketSession = "hz_session"

	subjectInternalNew    = "HZ.internal.sessions.new"
	subjectInternalGet    = "HZ.internal.sessions.get"
	subjectInternalDelete = "HZ.internal.sessions.delete"
)

var (
	ErrAuthenticationMissing = &hz.Error{
		Status:  http.StatusBadRequest,
		Message: "missing authentication header",
	}
	ErrInvalidCredentials = &hz.Error{
		Status:  http.StatusUnauthorized,
		Message: "invalid credentials",
	}
	ErrForbidden = &hz.Error{
		Status:  http.StatusForbidden,
		Message: "forbidden",
	}
)

func Start(ctx context.Context, nc *nats.Conn) (*Store, error) {
	s := &Store{
		Conn: nc,
	}
	if err := s.Start(ctx); err != nil {
		return nil, fmt.Errorf("start store: %w", err)
	}
	return s, nil
}

type Store struct {
	Conn *nats.Conn

	kv   jetstream.KeyValue
	subs []*nats.Subscription
}

func (s *Store) Start(ctx context.Context) error {
	kv, err := s.initSessionBucket(ctx)
	if err != nil {
		return fmt.Errorf("init session bucket: %w", err)
	}
	s.kv = kv

	//
	// New session.
	//
	sub, err := s.Conn.QueueSubscribe(
		subjectInternalNew,
		"sessions",
		func(msg *nats.Msg) {
			s.handleNewSession(ctx, msg)
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe new session: %w", err)
	}
	s.subs = append(s.subs, sub)

	//
	// Get session.
	//
	sub, err = s.Conn.QueueSubscribe(
		subjectInternalGet,
		"sessions",
		func(msg *nats.Msg) {
			s.handleGetSession(ctx, msg)
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe get session: %w", err)
	}
	s.subs = append(s.subs, sub)

	//
	// Delete session.
	//
	sub, err = s.Conn.QueueSubscribe(
		subjectInternalDelete,
		"sessions",
		func(msg *nats.Msg) {
			s.handleDeleteSession(ctx, msg)
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe delete session: %w", err)
	}
	s.subs = append(s.subs, sub)

	return nil
}

func (s *Store) Close() error {
	var errs error
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (s *Store) handleNewSession(ctx context.Context, msg *nats.Msg) {
	var user UserInfo
	if err := json.Unmarshal(msg.Data, &user); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: "new session: " + err.Error(),
		})
		return
	}
	// TODO: validate user a bit more?
	if user.Iss == "" || user.Sub == "" {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: "new session: missing iss or sub",
		})
		return
	}

	// TODO: use some long hash instead. Or?
	sessionID := uuid.New()
	if _, err := s.kv.Put(ctx, sessionID.String(), msg.Data); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "storing new session: " + err.Error(),
		})
		return
	}
	_ = hz.RespondOK(msg, []byte(sessionID.String()))
}

func (s *Store) handleGetSession(ctx context.Context, msg *nats.Msg) {
	auth := msg.Header.Get(hz.HeaderAuthorization)
	if auth == "" {
		_ = hz.RespondError(msg, ErrAuthenticationMissing)
		return
	}
	kve, err := s.kv.Get(ctx, auth)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			_ = hz.RespondError(msg, ErrInvalidCredentials)
			return
		}
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "get session: " + err.Error(),
		})
		return
	}
	_ = hz.RespondOK(msg, kve.Value())
}

func (s *Store) handleDeleteSession(ctx context.Context, msg *nats.Msg) {
	auth := msg.Header.Get(hz.HeaderAuthorization)
	if auth == "" {
		_ = hz.RespondError(msg, ErrAuthenticationMissing)
		return
	}
	if err := s.kv.Delete(ctx, auth); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "delete session: " + err.Error(),
		})
		return
	}
	_ = hz.RespondOK(msg, nil)
}

func (s *Store) initSessionBucket(
	ctx context.Context,
) (jetstream.KeyValue, error) {
	js, err := jetstream.New(s.Conn)
	if err != nil {
		return nil, fmt.Errorf("new jetstream: %w", err)
	}

	kv, err := js.KeyValue(ctx, bucketSession)
	if err != nil {
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return nil, fmt.Errorf(
				"get objects bucket %q: %w",
				bucketSession,
				err,
			)
		}
		kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Description: "KV bucket for storing horizon user sessions.",
			Bucket:      bucketSession,
			History:     1,
			TTL:         0,
		})
		if err != nil {
			return nil, fmt.Errorf(
				"create objects bucket %q: %w",
				bucketSession,
				err,
			)
		}
		return kv, nil
	}
	return kv, nil
}
