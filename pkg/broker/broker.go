package broker

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

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/verifa/horizon/pkg/auth"
	"github.com/verifa/horizon/pkg/hz"
)

// NATS headers that are not exported and hence we re-define them here
const (
	natsHeaderStatus             = "Status"
	natsHeaderStatusNoResponders = "503"
)

func Start(
	ctx context.Context,
	conn *nats.Conn,
	auth *auth.Auth,
) (*Broker, error) {
	b := &Broker{
		Conn: conn,
		Auth: auth,
	}
	if err := b.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting broker: %w", err)
	}
	return b, nil
}

type Broker struct {
	Conn *nats.Conn
	Auth *auth.Auth

	subscriptions []*nats.Subscription
}

func (b *Broker) Start(ctx context.Context) error {
	{
		sub, err := b.Conn.QueueSubscribe(
			hz.SubjectAPIBroker,
			"broker",
			func(msg *nats.Msg) {
				b.handleAPIMessage(ctx, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing to %q: %w", hz.SubjectAPIBroker, err)
		}
		b.subscriptions = append(b.subscriptions, sub)
		slog.Info("subscribed to", "subject", hz.SubjectAPIBroker)
	}
	{
		sub, err := b.Conn.QueueSubscribe(
			hz.SubjectInternalBroker,
			"broker",
			func(msg *nats.Msg) {
				b.handleInternalMessage(ctx, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing to %q: %w", hz.SubjectAPIBroker, err)
		}
		b.subscriptions = append(b.subscriptions, sub)
		slog.Info("subscribed to", "subject", hz.SubjectAPIBroker)
	}
	return nil
}

func (b *Broker) handleAPIMessage(ctx context.Context, msg *nats.Msg) {
	tokens := strings.Split(msg.Subject, ".")
	if len(tokens) != hz.SubjectInternalBrokerLength {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid subject: %s", msg.Subject),
		})
		return
	}
	key := hz.ObjectKey{
		Kind:    tokens[hz.SubjectInternalBrokerIndexKind],
		Group:   tokens[hz.SubjectInternalBrokerIndexGroup],
		Account: tokens[hz.SubjectInternalBrokerIndexAccount],
		Name:    tokens[hz.SubjectInternalBrokerIndexName],
	}

	ok, err := b.Auth.Check(ctx, auth.CheckRequest{
		Session: msg.Header.Get(hz.HeaderAuthorization),
		Verb:    auth.VerbRun,
		Object:  key,
	})
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

	b.handleInternalMessage(ctx, msg)
}

func (b *Broker) handleInternalMessage(ctx context.Context, msg *nats.Msg) {
	var runMsg hz.RunMsg
	if err := json.Unmarshal(msg.Data, &runMsg); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("unmarshalling run message: %s", err.Error()),
		})
		return
	}
	adMsg := hz.AdvertiseMsg{
		LabelSelector: runMsg.LabelSelector,
	}
	bAdMsg, err := json.Marshal(adMsg)
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("marshal advertise message: %s", err.Error()),
		})
		return
	}
	tokens := strings.Split(msg.Subject, ".")
	if len(tokens) != hz.SubjectInternalBrokerLength {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusBadRequest,
			Message: fmt.Sprintf("invalid subject: %s", msg.Subject),
		})
		return
	}

	key := hz.ObjectKey{
		Kind:    tokens[hz.SubjectInternalBrokerIndexKind],
		Group:   tokens[hz.SubjectInternalBrokerIndexGroup],
		Account: tokens[hz.SubjectInternalBrokerIndexAccount],
		Name:    tokens[hz.SubjectInternalBrokerIndexName],
	}
	action := tokens[hz.SubjectInternalBrokerIndexAction]

	advertiseSubject := fmt.Sprintf(
		hz.SubjectActorAdvertiseFmt,
		key.Group,
		key.Kind,
		key.Account,
		key.Name,
		action,
	)
	inbox := nats.NewInbox()

	msgCh := make(chan *nats.Msg, 100)
	sub, err := b.Conn.ChanSubscribe(inbox, msgCh)
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("subscribing to %s: %s", inbox, err.Error()),
		})
		return
	}
	defer func() {
		_ = sub.Unsubscribe()
		close(msgCh)
	}()

	// PublishRequest works like Request but returns immediately.
	// It is up to us to listen for the responses.
	//
	// If there are no responders (i.e. subscribers) for the request,
	// nats automatically adds one reply with a "Status" header of 503.
	if err := b.Conn.PublishRequest(advertiseSubject, inbox, bAdMsg); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"publishing request to %s: %s",
				advertiseSubject,
				err.Error(),
			),
		})
		return
	}
	// processMessages waits for either the first message (from an actor) or a
	// timeout, and then returns the reply.
	processMessages := func() *nats.Msg {
		for {
			select {
			case reply := <-msgCh:
				return reply
			case <-time.After(time.Second * 1):
				return nil
			}
		}
	}
	adReply := processMessages()
	if adReply == nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusServiceUnavailable,
			Message: "no actors responded to advertise request",
		})
		return
	}
	// Check for any headers added by nats.
	if adReply.Header.Get(natsHeaderStatus) == natsHeaderStatusNoResponders {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusServiceUnavailable,
			Message: "no actors responded to advertise request",
		})
		return
	}

	// Check the status header set/expected by horizon.
	status, err := strconv.Atoi(adReply.Header.Get(hz.HeaderStatus))
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("parsing status header: %s", err.Error()),
		})
		return
	}
	if status != http.StatusOK {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  status,
			Message: string(adReply.Data),
		})
		return
	}
	id, err := uuid.ParseBytes(adReply.Data)
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("parsing actor uuid: %s", err.Error()),
		})
		return
	}

	runSubject := fmt.Sprintf(
		hz.SubjectActorRunFmt,
		key.Group,
		key.Kind,
		key.Account,
		key.Name,
		action,
		id,
	)
	// Remove some time from the timeout to account for the time it takes
	// to advertise the action.
	// We do not want the client to timeout before the broker does.
	timeout := runMsg.Timeout - (time.Second * 2)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	runReply, err := b.Conn.RequestWithContext(ctx, runSubject, runMsg.Data)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders):
			_ = hz.RespondError(msg, &hz.Error{
				Status:  http.StatusServiceUnavailable,
				Message: "actor id: " + id.String(),
			})
			return
		case errors.Is(err, context.DeadlineExceeded),
			errors.Is(err, nats.ErrTimeout):
			_ = hz.RespondError(msg, &hz.Error{
				Status:  http.StatusRequestTimeout,
				Message: "actor id: " + id.String(),
			})
			return
		default:
			_ = hz.RespondError(msg, &hz.Error{
				Status:  http.StatusInternalServerError,
				Message: "actor id: " + id.String() + ": " + err.Error(),
			})
			return
		}
	}
	// Forward the reply message onto the original caller.
	_ = msg.RespondMsg(runReply)
}

func (b *Broker) Stop() error {
	var errs error
	for _, sub := range b.subscriptions {
		errs = errors.Join(errs, sub.Unsubscribe())
	}
	return errs
}
