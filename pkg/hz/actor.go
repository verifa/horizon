package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
)

const ActorSubjectHTTPRender = "actor.%s.http.render"

type Actioner interface {
	Action() string
}

type Action[T Objecter] interface {
	Actioner
	Do(context.Context, T) (T, error)
}

type ActorOption[T Objecter] func(*actorOption[T])

func WithActorActioner[T Objecter](
	actioner Action[T],
) ActorOption[T] {
	return func(ro *actorOption[T]) {
		ro.actioners = append(ro.actioners, actioner)
	}
}

func WithActorLabels[T Objecter](
	labels map[string]string,
) ActorOption[T] {
	return func(ro *actorOption[T]) {
		ro.labels = labels
	}
}

type actorOption[T Objecter] struct {
	actioners []Action[T]

	forObject T
	labels    map[string]string
}

func StartActor[T Objecter](
	ctx context.Context,
	nc *nats.Conn,
	opts ...ActorOption[T],
) (*Actor[T], error) {
	a := Actor[T]{
		nc: nc,
	}
	if err := a.Start(ctx, opts...); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}
	return &a, nil
}

type Actor[T Objecter] struct {
	nc *nats.Conn

	subscriptions []*nats.Subscription
}

func (a *Actor[T]) Start(ctx context.Context, opts ...ActorOption[T]) error {
	opt := actorOption[T]{}
	for _, o := range opts {
		o(&opt)
	}
	// Setup common labels.
	if err := a.addCommonLabels(&opt); err != nil {
		return fmt.Errorf("add common labels: %w", err)
	}

	id := uuid.New()
	for _, actioner := range opt.actioners {
		if err := a.startActioner(ctx, actioner, opt, id); err != nil {
			return fmt.Errorf("start actioner: %w", err)
		}
	}
	return nil
}

func (a *Actor[T]) addCommonLabels(opt *actorOption[T]) error {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}
	if opt.labels == nil {
		opt.labels = make(map[string]string)
	}
	opt.labels["hostname"] = hostname
	opt.labels["os"] = runtime.GOOS
	opt.labels["arch"] = runtime.GOARCH
	return nil
}

func (a *Actor[T]) startActioner(
	ctx context.Context,
	actioner Action[T],
	opt actorOption[T],
	id uuid.UUID,
) error {
	group := opt.forObject.ObjectGroup()
	version := opt.forObject.ObjectVersion()
	kind := opt.forObject.ObjectKind()
	action := actioner.Action()
	advertiseSubject := fmt.Sprintf(
		SubjectActorAdvertise,
		group,
		version,
		kind,
		action,
	)
	runSubject := fmt.Sprintf(SubjectActorRun, group, version, kind, action, id)

	adSub, err := a.nc.Subscribe(
		advertiseSubject,
		func(msg *nats.Msg) {
			var adMsg AdvertiseMsg
			if err := json.Unmarshal(msg.Data, &adMsg); err != nil {
				slog.Error("unmarshal advertise message", "error", err)
				_ = RespondError(
					msg,
					&Error{
						Status: http.StatusBadRequest,
						Message: fmt.Sprintf(
							"unmarshal advertise message: %s",
							err,
						),
					},
				)
				return
			}

			// If the label selector doesn't match, ignore the message and don't
			// respond.
			if !adMsg.LabelSelector.Matches(opt.labels) {
				return
			}
			if err := RespondOK(msg, []byte(id.String())); err != nil {
				slog.Error("responding to advertise message", "error", err)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe advertiser: %w", err)
	}
	a.subscriptions = append(a.subscriptions, adSub)
	slog.Info("subscribed to advertise", "subject", advertiseSubject)

	doSub, err := a.nc.Subscribe(
		runSubject,
		func(msg *nats.Msg) {
			var t T
			if err := json.Unmarshal(msg.Data, &t); err != nil {
				slog.Error("unmarshalling msg data", "error", err)
				_ = RespondError(
					msg,
					&Error{
						Status: http.StatusBadRequest,
						Message: fmt.Sprintf(
							"unmarshalling msg data: %s",
							err,
						),
					},
				)
				return
			}
			resp, err := actioner.Do(ctx, t)
			if err != nil {
				_ = RespondError(
					msg,
					&Error{
						Status: http.StatusInternalServerError,
						Message: fmt.Sprintf(
							"running action: %s",
							err,
						),
					},
				)
				return
			}
			b, err := json.Marshal(resp)
			if err != nil {
				slog.Error("marshalling response", "error", err)
				_ = RespondError(
					msg,
					&Error{
						Status: http.StatusInternalServerError,
						Message: fmt.Sprintf(
							"marshalling response: %s",
							err,
						),
					},
				)
				return
			}
			_ = RespondOK(msg, b)
		},
	)
	if err != nil {
		return fmt.Errorf("subscribe action handler: %w", err)
	}
	a.subscriptions = append(a.subscriptions, doSub)
	slog.Info("subscribed to run", "subject", runSubject)

	return nil
}

func (a *Actor[T]) Stop() error {
	var errs error
	for _, sub := range a.subscriptions {
		err := sub.Unsubscribe()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}
