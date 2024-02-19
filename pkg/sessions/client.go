package sessions

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/hz"
)

func WithSessionFromMsg(msg *nats.Msg) SessionOption {
	return func(o *sessionOptions) {
		o.msg = msg
	}
}

func WithSessionID(sessionID string) SessionOption {
	return func(o *sessionOptions) {
		o.sessionID = &sessionID
	}
}

type SessionOption func(*sessionOptions)

type sessionOptions struct {
	msg       *nats.Msg
	sessionID *string
}

func Get(
	ctx context.Context,
	conn *nats.Conn,
	opts ...SessionOption,
) (UserInfo, error) {
	o := &sessionOptions{}
	for _, opt := range opts {
		opt(o)
	}

	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req := nats.NewMsg(subjectInternalGet)
	if o.sessionID != nil {
		req.Header = make(nats.Header)
		req.Header.Add(hz.HeaderAuthorization, *o.sessionID)
	}
	if o.msg != nil {
		req.Header = o.msg.Header
	}
	reply, err := conn.RequestMsgWithContext(ctx, req)
	if err != nil {
		// TODO: handle error.
		return UserInfo{}, fmt.Errorf("request auth: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(hz.HeaderStatus))
	if err != nil {
		return UserInfo{}, fmt.Errorf("invalid status in reply: %w", err)
	}
	if status == 200 {
		var user UserInfo
		if err := json.Unmarshal(reply.Data, &user); err != nil {
			return UserInfo{}, fmt.Errorf("unmarshal user reply: %w", err)
		}
		return user, nil
	}

	return UserInfo{}, &hz.Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func New(
	ctx context.Context,
	conn *nats.Conn,
	user UserInfo,
) (string, error) {
	bUser, err := json.Marshal(user)
	if err != nil {
		return "", fmt.Errorf("marshal user: %w", err)
	}
	msg := nats.NewMsg(subjectInternalNew)
	msg.Data = bUser
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		// TODO: handle error.
		return "", fmt.Errorf("request new session: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(hz.HeaderStatus))
	if err != nil {
		return "", fmt.Errorf("invalid status: %w", err)
	}
	if status == 200 {
		return string(reply.Data), nil
	}
	return "", &hz.Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func Delete(
	ctx context.Context,
	conn *nats.Conn,
	opts ...SessionOption,
) error {
	o := &sessionOptions{}
	for _, opt := range opts {
		opt(o)
	}
	req := nats.NewMsg(subjectInternalDelete)
	if o.sessionID != nil {
		req.Header = make(nats.Header)
		req.Header.Add(hz.HeaderAuthorization, *o.sessionID)
	}
	if o.msg != nil {
		req.Header = o.msg.Header
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := conn.RequestMsgWithContext(ctx, req)
	if err != nil {
		// TODO: handle error.
		return fmt.Errorf("request delete session: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(hz.HeaderStatus))
	if err != nil {
		return fmt.Errorf("invalid status: %w", err)
	}
	if status == 200 {
		return nil
	}
	return &hz.Error{
		Status:  status,
		Message: string(reply.Data),
	}
}
