package auth

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
)

const (
	// GroupSystemAuthenticated is a group all users with a session belong to.
	// It is a way of identifying "anyone who is authenticated".
	GroupSystemAuthenticated = "system:authenticated"
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

type Sessions struct {
	Conn *nats.Conn

	kv jetstream.KeyValue
}

func (s *Sessions) Start(ctx context.Context) error {
	kv, err := s.initSessionBucket(ctx)
	if err != nil {
		return fmt.Errorf("init session bucket: %w", err)
	}
	s.kv = kv

	return nil
}

func (s *Sessions) New(ctx context.Context, user UserInfo) (string, error) {
	// TODO: validate user a bit more?
	if user.Iss == "" || user.Sub == "" {
		return "", &hz.Error{
			Status:  http.StatusBadRequest,
			Message: "new session: missing iss or sub",
		}
	}

	data, err := json.Marshal(user)
	if err != nil {
		return "", &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "new session: marshal user: " + err.Error(),
		}
	}
	// TODO: use some long hash instead. Or?
	sessionID := uuid.New()
	if _, err := s.kv.Put(ctx, sessionID.String(), data); err != nil {
		return "", &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "storing new session: " + err.Error(),
		}
	}
	return sessionID.String(), nil
}

func (s *Sessions) Get(ctx context.Context, session string) (UserInfo, error) {
	if session == "" {
		return UserInfo{}, ErrAuthenticationMissing
	}
	kve, err := s.kv.Get(ctx, session)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return UserInfo{}, ErrInvalidCredentials
		}
		return UserInfo{}, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "get session: " + err.Error(),
		}
	}
	var user UserInfo
	if err := json.Unmarshal(kve.Value(), &user); err != nil {
		return UserInfo{}, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "unmarshal user: " + err.Error(),
		}
	}
	// Add default groups.
	user.Groups = append(user.Groups, GroupSystemAuthenticated)
	return user, nil
}

func (s *Sessions) Delete(ctx context.Context, session string) error {
	if err := s.kv.Delete(ctx, session); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) ||
			errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		return &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "delete session: " + err.Error(),
		}
	}
	return nil
}

func (s *Sessions) initSessionBucket(
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
