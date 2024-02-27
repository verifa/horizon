package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/hz"
)

const (
	// Format: HZ.<internal/api>.store.<command>.<group>.<kind>.<account>.<name>

	subjectInternalStore = "HZ.internal.store.*.*.*.*.*"
	subjectAPIStore      = "HZ.api.store.*.*.*.*.*"
	subjectIndexCommand  = 3
	subjectIndexGroup    = 4
	subjectIndexKind     = 5
	subjectIndexAccount  = 6
	subjectIndexName     = 7
	subjectLength        = 8
)

type StoreCommand string

const (
	StoreCommandCreate StoreCommand = "create"
	StoreCommandApply  StoreCommand = "apply"
	StoreCommandGet    StoreCommand = "get"
	StoreCommandList   StoreCommand = "list"
	StoreCommandDelete StoreCommand = "delete"
)

func (c StoreCommand) String() string {
	return string(c)
}

type Store struct {
	Conn *nats.Conn
	Auth *auth.Auth

	js    jetstream.JetStream
	kv    jetstream.KeyValue
	mutex jetstream.KeyValue
	subs  []*nats.Subscription
}

func (s Store) Close() error {
	var errs error
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

type StoreOption func(*storeOptions)

func WithMutexTTL(ttl time.Duration) StoreOption {
	return func(o *storeOptions) {
		o.mutexTTL = ttl
	}
}

var defaultStoreOptions = storeOptions{
	mutexTTL: time.Minute,
}

type storeOptions struct {
	mutexTTL time.Duration
}

func StartStore(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
	opts ...StoreOption,
) (*Store, error) {
	opt := defaultStoreOptions
	for _, o := range opts {
		o(&opt)
	}

	store := Store{
		Conn: conn,
		Auth: auth,
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

	{
		sub, err := conn.QueueSubscribe(
			subjectInternalStore,
			"store",
			func(msg *nats.Msg) {
				slog.Info("received store message", "subject", msg.Subject)
				store.handleInternalMsg(ctx, msg)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("subscribing store: %w", err)
		}
		store.subs = append(store.subs, sub)
	}
	{
		sub, err := conn.QueueSubscribe(
			subjectAPIStore,
			"store",
			func(msg *nats.Msg) {
				slog.Info("received store message", "subject", msg.Subject)
				store.handleAPIMsg(ctx, msg)
			},
		)
		if err != nil {
			return nil, fmt.Errorf("subscribing store: %w", err)
		}
		store.subs = append(store.subs, sub)
	}

	return &store, nil
}

func InitKeyValue(
	ctx context.Context,
	conn *nats.Conn,
	opts ...StoreOption,
) error {
	opt := defaultStoreOptions
	for _, o := range opts {
		o(&opt)
	}

	js, err := jetstream.New(conn)
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

func (s Store) handleAPIMsg(ctx context.Context, msg *nats.Msg) {
	// Parse subject to get details.
	parts := strings.Split(msg.Subject, ".")
	if len(parts) != subjectLength {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid subject: %q", msg.Subject),
		})
		return
	}
	cmd := StoreCommand(parts[subjectIndexCommand])

	key := hz.ObjectKey{
		Name:    parts[subjectIndexName],
		Account: parts[subjectIndexAccount],
		Kind:    parts[subjectIndexKind],
		Group:   parts[subjectIndexGroup],
	}

	req := auth.CheckRequest{
		Session: msg.Header.Get(hz.HeaderAuthorization),
		Object:  key,
	}
	switch cmd {
	case StoreCommandList:
		// List is a bit special. We do not check for permissions to a specific
		// object, instead we want to list the objects which we can access.
		// The internal msg handler will do the actual listing and honour the
		// session.
		// Just forward it there...
		// Check session is valid, at the very least.
		session := msg.Header.Get(hz.HeaderAuthorization)
		_, err := s.Auth.Sessions.Get(ctx, session)
		if err != nil {
			_ = hz.RespondError(msg, err)
			return
		}

		s.handleInternalMsg(ctx, msg)
		return
	case StoreCommandGet:
		req.Verb = auth.VerbRead
	case StoreCommandCreate:
		req.Verb = auth.VerbCreate
	case StoreCommandApply:
		// This requires checking if it's a create or edit operation.
		_, err := s.get(ctx, key)
		if errors.Is(err, hz.ErrNotFound) {
			req.Verb = auth.VerbCreate
		} else {
			req.Verb = auth.VerbUpdate
		}
	case StoreCommandDelete:
		req.Verb = auth.VerbDelete
	default:
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid command: %q", cmd),
		})
		return

	}
	ok, err := s.Auth.Check(ctx, req)
	if err != nil {
		_ = hz.RespondError(msg, err)
		return
	}
	if !ok {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusForbidden,
			Message: "forbidden",
		})
		return
	}
	s.handleInternalMsg(ctx, msg)
}

// handleInternalMsg handles messages for the internal (unprotected) nats
// subjects.
// TODO: internal messages still honour the user's authorization (if present).
func (s Store) handleInternalMsg(ctx context.Context, msg *nats.Msg) {
	// Parse subject to get details.
	parts := strings.Split(msg.Subject, ".")
	if len(parts) != subjectLength {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid subject: %q", msg.Subject),
		})
		return
	}
	cmd := StoreCommand(parts[subjectIndexCommand])

	key := hz.ObjectKey{
		Name:    parts[subjectIndexName],
		Account: parts[subjectIndexAccount],
		Kind:    parts[subjectIndexKind],
		Group:   parts[subjectIndexGroup],
	}

	data, err := removeReadOnlyFields(msg.Data)
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: "deleting read-only fields: " + err.Error(),
		})
		return
	}

	switch cmd {
	case StoreCommandCreate:
		req := CreateRequest{
			Key:  key,
			Data: data,
		}
		if err := s.Create(ctx, req); err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
	case StoreCommandApply:
		manager := msg.Header.Get(hz.HeaderApplyFieldManager)
		forceStr := msg.Header.Get(hz.HeaderApplyForceConflicts)
		force, err := strconv.ParseBool(forceStr)
		if err != nil {
			_ = hz.RespondError(
				msg,
				&hz.Error{
					Status:  http.StatusBadRequest,
					Message: "invalid header " + hz.HeaderApplyForceConflicts,
				},
			)
			return
		}
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
			Data:    data,
			Manager: manager,
			Key:     key,
			Force:   force,
		}

		if err := s.Apply(ctx, req); err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
	case StoreCommandGet:
		req := GetRequest{
			Key: key,
		}
		resp, err := s.Get(ctx, req)
		if err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
		_ = hz.RespondOK(msg, resp)
	case StoreCommandList:
		// Logic: the auth rbac does not know which objects exist.
		// Therefore, we cannot ask it which objects we can list.
		// Therefore, list all the actual objects that match the supplied key,
		// and then filter them with rbac.
		req := ListRequest{
			Key: key,
		}
		resp, err := s.List(ctx, req)
		if err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
		session := msg.Header.Get(hz.HeaderAuthorization)
		if session != "" {
			// Filter the response with the rbac.
			if err := s.Auth.List(ctx, auth.ListRequest{
				Session:    msg.Header.Get(hz.HeaderAuthorization),
				ObjectList: resp,
			}); err != nil {
				_ = hz.RespondError(msg, err)
				return
			}
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

func removeReadOnlyFields(data []byte) ([]byte, error) {
	var errs error
	var err error
	data, err = sjson.DeleteBytes(data, "metadata.revision")
	errs = errors.Join(errs, err)
	data, err = sjson.DeleteBytes(data, "metadata.managedFields")
	errs = errors.Join(errs, err)

	return data, errs
}
