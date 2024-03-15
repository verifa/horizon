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
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/tidwall/sjson"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/hz"
)

const (
	// Format: HZ.<internal/api>.store.<command>.<group>.<kind>.<account>.<name>

	subjectInternalStore = "HZ.internal.store.*.*.*.*.*.*"
	subjectAPIStore      = "HZ.api.store.*.*.*.*.*.*"
	subjectIndexCommand  = 3
	subjectIndexGroup    = 4
	subjectIndexVersion  = 5
	subjectIndexKind     = 6
	subjectIndexAccount  = 7
	subjectIndexName     = 8
	subjectLength        = 9
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

type StoreOption func(*storeOptions)

func WithMutexTTL(ttl time.Duration) StoreOption {
	return func(o *storeOptions) {
		o.mutexTTL = ttl
	}
}

func WithStopTimeout(timeout time.Duration) StoreOption {
	return func(o *storeOptions) {
		o.stopTimeout = timeout
	}
}

var defaultStoreOptions = storeOptions{
	mutexTTL:    time.Minute,
	stopTimeout: time.Minute,
}

type storeOptions struct {
	mutexTTL    time.Duration
	stopTimeout time.Duration
}

func StartStore(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
	opts ...StoreOption,
) (*Store, error) {
	store := Store{
		Conn: conn,
		Auth: auth,
	}
	if err := store.Start(ctx, conn, auth, opts...); err != nil {
		return nil, fmt.Errorf("starting store: %w", err)
	}
	return &store, nil
}

type Store struct {
	Conn *nats.Conn
	Auth *auth.Auth

	js    jetstream.JetStream
	kv    jetstream.KeyValue
	mutex jetstream.KeyValue
	gc    *GarbageCollector
	subs  []*nats.Subscription

	stopTimeout time.Duration
	wg          sync.WaitGroup
}

func (s *Store) Start(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
	opts ...StoreOption,
) error {
	opt := defaultStoreOptions
	for _, o := range opts {
		o(&opt)
	}

	s.stopTimeout = opt.stopTimeout

	js, err := jetstream.New(conn)
	if err != nil {
		return fmt.Errorf("new jetstream: %w", err)
	}
	s.js = js
	kv, err := js.KeyValue(ctx, hz.BucketObjects)
	if err != nil {
		return fmt.Errorf(
			"conntecting to objects kv bucket %q: %w",
			hz.BucketObjects,
			err,
		)
	}
	s.kv = kv
	mutex, err := js.KeyValue(ctx, hz.BucketMutex)
	if err != nil {
		return fmt.Errorf(
			"connecting to mutex kv bucket %q: %w",
			hz.BucketMutex,
			err,
		)
	}
	s.mutex = mutex

	{
		sub, err := conn.QueueSubscribe(
			subjectInternalStore,
			"store",
			func(msg *nats.Msg) {
				slog.Info("received store message", "subject", msg.Subject)
				go s.handleInternalMsg(ctx, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing store: %w", err)
		}
		s.subs = append(s.subs, sub)
	}
	{
		sub, err := conn.QueueSubscribe(
			subjectAPIStore,
			"store",
			func(msg *nats.Msg) {
				slog.Info("received store message", "subject", msg.Subject)
				go s.handleAPIMsg(ctx, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing store: %w", err)
		}
		s.subs = append(s.subs, sub)
	}

	// Start garbage collector.
	gc := &GarbageCollector{
		Conn: conn,
		KV:   kv,
	}
	if err := gc.Start(ctx); err != nil {
		return fmt.Errorf("start garbage collector: %w", err)
	}
	s.gc = gc
	return nil
}

func (s *Store) Close() error {
	var errs error
	for _, sub := range s.subs {
		if err := sub.Unsubscribe(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	s.gc.Stop()

	// Wait for all store operations to finish, or timeout.
	if s.stopWaitTimeout() {
		errs = errors.Join(
			errs,
			fmt.Errorf(
				"timeout after %s waiting for store operations to finish",
				s.stopTimeout,
			),
		)
	}
	return errs
}

func (s *Store) stopWaitTimeout() bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.wg.Wait()
	}()
	tickDuration := time.Second * 10
	ticker := time.NewTicker(tickDuration)
	elapsedTime := time.Duration(0)
	for {
		elapsedTime += tickDuration
		select {
		case <-ticker.C:
			slog.Info(
				"waiting for reconcile loops to finish",
				"elapsed",
				elapsedTime,
				"timeout",
				s.stopTimeout,
			)
		case <-done:
			return false // completed normally
		case <-time.After(s.stopTimeout):
			return true // timed out
		}
	}
}

func (s *Store) handleAPIMsg(ctx context.Context, msg *nats.Msg) {
	s.wg.Add(1)
	defer s.wg.Done()
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
		Group:   parts[subjectIndexGroup],
		Version: parts[subjectIndexVersion],
		Kind:    parts[subjectIndexKind],
		Account: parts[subjectIndexAccount],
		Name:    parts[subjectIndexName],
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
// Even though it is unprotected, some commands (like list) still honour the
// authz for a user session, if provided.
func (s *Store) handleInternalMsg(ctx context.Context, msg *nats.Msg) {
	s.wg.Add(1)
	defer s.wg.Done()
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
		Group:   parts[subjectIndexGroup],
		Version: parts[subjectIndexVersion],
		Kind:    parts[subjectIndexKind],
		Account: parts[subjectIndexAccount],
		Name:    parts[subjectIndexName],
	}

	switch cmd {
	case StoreCommandCreate:
		req := CreateRequest{
			Key:  key,
			Data: msg.Data,
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
			Data:    msg.Data,
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
		req := DeleteRequest{
			Key: key,
		}
		if err := s.Delete(ctx, req); err != nil {
			_ = hz.RespondError(msg, err)
			return
		}
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
	return sjson.DeleteBytes(data, "metadata.revision")
}
