package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/hz"
)

const (
	// Format: STORE.<command>.<kind>.<account>.<name>
	subjectStore        = "STORE.*.*.*.*"
	subjectIndexCommand = 1
	subjectIndexKind    = 2
	subjectIndexAccount = 3
	subjectIndexName    = 4
)

type StoreCommand string

const (
	StoreCommandCreate StoreCommand = "create"
	StoreCommandApply  StoreCommand = "apply"
	StoreCommandGet    StoreCommand = "get"
	StoreCommandList   StoreCommand = "list"
	StoreCommandUpdate StoreCommand = "update"
	StoreCommandDelete StoreCommand = "delete"
)

func (c StoreCommand) String() string {
	return string(c)
}

type Store struct {
	conn  *nats.Conn
	js    jetstream.JetStream
	kv    jetstream.KeyValue
	mutex jetstream.KeyValue
	sub   *nats.Subscription
}

func (s Store) Close() error {
	return s.sub.Unsubscribe()
}

type StoreOption func(*storeOptions)

func WithMutexTTL(ttl time.Duration) StoreOption {
	return func(o *storeOptions) {
		o.mutexTTL = ttl
	}
}

type storeOptions struct {
	mutexTTL time.Duration
}

func StartStore(
	ctx context.Context,
	conn *nats.Conn,
	opts ...StoreOption,
) (*Store, error) {
	opt := storeOptions{
		mutexTTL: time.Minute,
	}
	for _, o := range opts {
		o(&opt)
	}

	store := Store{
		conn: conn,
	}

	if err := store.initKVBuckets(ctx, opt); err != nil {
		return nil, fmt.Errorf("init kv buckets: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		return nil, fmt.Errorf("new jetstream: %w", err)
	}
	store.js = js
	kv, err := js.KeyValue(ctx, hz.BucketObjects)
	if err != nil {
		return nil, fmt.Errorf(
			"conntecting to objects kv bucket %q: %w",
			hz.BucketObjects,
			err,
		)
	}
	store.kv = kv
	mutex, err := js.KeyValue(ctx, hz.BucketMutex)
	if err != nil {
		return nil, fmt.Errorf(
			"connecting to mutex kv bucket %q: %w",
			hz.BucketMutex,
			err,
		)
	}
	store.mutex = mutex

	sub, err := conn.QueueSubscribe(
		"STORE.*.*.*.*",
		"store",
		func(msg *nats.Msg) {
			slog.Info("received store message", "msg", msg)
			store.handleStoreMsg(ctx, msg)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("subscribing store: %w", err)
	}
	store.sub = sub
	return &store, nil
}

func (s Store) initKVBuckets(ctx context.Context, opt storeOptions) error {
	js, err := jetstream.New(s.conn)
	if err != nil {
		return fmt.Errorf("new jetstream: %w", err)
	}

	if _, err := js.KeyValue(ctx, hz.BucketObjects); err != nil {
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return fmt.Errorf(
				"get objects bucket %q: %w",
				hz.BucketObjects,
				err,
			)
		}
		if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Description: "KV bucket for storing horizon objects.",
			Bucket:      hz.BucketObjects,
			History:     1,
			TTL:         0,
		}); err != nil {
			return fmt.Errorf(
				"create objects bucket %q: %w",
				hz.BucketObjects,
				err,
			)
		}
	}
	// TODO: handle updating the objects bucket if it exists.

	if _, err := js.KeyValue(ctx, hz.BucketMutex); err != nil {
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return fmt.Errorf(
				"get mutex bucket %q for %q: %w",
				hz.BucketMutex,
				hz.BucketObjects,
				err,
			)
		}
		if _, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      hz.BucketMutex,
			Description: "Mutex for " + hz.BucketObjects,
			History:     1,
			// In case unlocking fails, or there's a serious error,
			// NATS will automatically unlock the mutex after the TTL.
			// Behind the scenes, NATS will delete the TTL value,
			// which from the mutex's perspective means there is no lock.
			TTL: opt.mutexTTL,
		}); err != nil {
			return fmt.Errorf(
				"create mutex bucket %q for %q: %w",
				hz.BucketMutex,
				hz.BucketObjects,
				err,
			)
		}
	}
	// TODO: handle updating the mutex bucket if it exists.
	return nil
}

func (s Store) handleStoreMsg(ctx context.Context, msg *nats.Msg) {
	// Parse subject to get details.
	parts := strings.Split(msg.Subject, ".")
	if len(parts) != 5 {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: "invalid subject",
		})
	}
	cmd := StoreCommand(parts[subjectIndexCommand])
	kind := parts[subjectIndexKind]
	account := parts[subjectIndexAccount]
	name := parts[subjectIndexName]

	switch cmd {
	case StoreCommandCreate:
		req := CreateRequest{
			Key:  hz.KeyForObjectParams(kind, account, name),
			Data: msg.Data,
		}
		if err := s.Create(ctx, req); err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
	case StoreCommandApply:
		manager := msg.Header.Get(hz.HeaderFieldManager)
		if manager == "" {
			_ = hz.RespondError(
				msg,
				&hz.Error{
					Status:  http.StatusBadRequest,
					Message: "missing field manager",
				},
			)
			return
		}
		req := ApplyRequest{
			Data:    msg.Data,
			Manager: manager,
			Kind:    kind,
			Key:     hz.KeyForObjectParams(kind, account, name),
		}

		if err := s.Apply(ctx, req); err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
	case StoreCommandUpdate:
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusNotImplemented,
			Message: "todo: not implemented",
		})
		return
	case StoreCommandGet:
		req := GetRequest{
			Key: hz.KeyForObjectParams(kind, account, name),
		}
		resp, err := s.Get(ctx, req)
		if err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
		_ = hz.RespondOK(msg, resp)
	case StoreCommandList:
		req := ListRequest{
			Key: hz.KeyForObjectParams(kind, account, name),
		}
		resp, err := s.List(ctx, req)
		if err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
		data, err := json.Marshal(resp)
		if err != nil {
			_ = hz.RespondError(msg, &hz.Error{
				Status:  http.StatusInternalServerError,
				Message: "marshalling list response: " + err.Error(),
			})
			return
		}
		_ = hz.RespondOK(msg, data)
		return
	case StoreCommandDelete:
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusNotImplemented,
			Message: "todo: not implemented",
		})
		return
	default:
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: "invalid command",
		})
		return
	}

	_ = hz.RespondOK(msg, nil)
}
