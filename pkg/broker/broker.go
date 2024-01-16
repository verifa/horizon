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
	"github.com/verifa/horizon/pkg/hz"
)

// NATS headers that are not exported and hence we re-define them here
const (
	natsHeaderStatus             = "Status"
	natsHeaderStatusNoResponders = "503"
)

func StartBroker(ctx context.Context, nc *nats.Conn) (*Broker, error) {
	b := &Broker{
		Conn: nc,
	}
	if err := b.Start(ctx); err != nil {
		return nil, fmt.Errorf("starting broker: %w", err)
	}
	return b, nil
}

type Broker struct {
	Conn *nats.Conn

	subscription *nats.Subscription
}

func (b *Broker) Start(ctx context.Context) error {
	sub, err := b.Conn.QueueSubscribe(
		hz.SubjectBroker,
		"broker",
		func(msg *nats.Msg) {
			if err := b.handleMessage(msg); err != nil {
				slog.Error("handling message", "error", err)
			}
		},
	)
	if err != nil {
		return fmt.Errorf("subscribing to %q: %w", hz.SubjectBroker, err)
	}
	b.subscription = sub
	return nil
}

func (b *Broker) handleMessage(msg *nats.Msg) error {
	var runMsg hz.RunMsg
	if err := json.Unmarshal(msg.Data, &runMsg); err != nil {
		return fmt.Errorf("unmarshal run message data: %w", err)
	}
	adMsg := hz.AdvertiseMsg{
		LabelSelector: runMsg.LabelSelector,
	}
	bAdMsg, err := json.Marshal(adMsg)
	if err != nil {
		return fmt.Errorf("marshal advertise message: %w", err)
	}
	tokens := strings.Split(msg.Subject, ".")
	kind := tokens[1]
	account := tokens[2]
	name := tokens[3]
	action := tokens[4]

	advertiseSubject := fmt.Sprintf(
		hz.SubjectActorAdvertiseFmt,
		kind,
		account,
		name,
		action,
	)
	inbox := nats.NewInbox()

	msgCh := make(chan *nats.Msg, 100)
	sub, err := b.Conn.ChanSubscribe(inbox, msgCh)
	if err != nil {
		return fmt.Errorf("subscribing to %s: %w", inbox, err)
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
		return fmt.Errorf("publishing request to %s: %w", advertiseSubject, err)
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
		return hz.RespondError(
			msg,
			&hz.Error{
				Status:  http.StatusServiceUnavailable,
				Message: "no actors responded to advertise request",
			},
		)
	}
	// Check for any headers added by nats.
	if adReply.Header.Get(natsHeaderStatus) == natsHeaderStatusNoResponders {
		return hz.RespondError(
			msg,
			&hz.Error{
				Status:  http.StatusServiceUnavailable,
				Message: "no actors responded to advertise request",
			},
		)
	}

	// Check the status header set/expected by horizon.
	status, err := strconv.Atoi(adReply.Header.Get(hz.HeaderStatus))
	if err != nil {
		return hz.RespondError(msg, &hz.Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("parsing status header: %s", err.Error()),
		})
	}
	if status != http.StatusOK {
		return hz.RespondError(msg, &hz.Error{
			Status:  status,
			Message: string(adReply.Data),
		})
	}
	id, err := uuid.ParseBytes(adReply.Data)
	if err != nil {
		return hz.RespondError(
			msg,
			&hz.Error{
				Status:  http.StatusInternalServerError,
				Message: fmt.Sprintf("parsing actor uuid: %s", err.Error()),
			},
		)
	}

	runSubject := fmt.Sprintf(
		hz.SubjectActorRunFmt,
		kind,
		account,
		name,
		action,
		id,
	)
	// Remove some time from the timeout to account for the time it takes
	// to advertise the action.
	// We do not want the client to timeout before the broker does.
	timeout := runMsg.Timeout - (time.Second * 2)
	runReply, err := b.Conn.Request(runSubject, runMsg.Data, timeout)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders):
			return hz.RespondError(
				msg,
				&hz.Error{
					Status:  http.StatusServiceUnavailable,
					Message: "actor id: " + id.String(),
				},
			)
		case errors.Is(err, nats.ErrTimeout):
			return hz.RespondError(
				msg,
				&hz.Error{
					Status:  http.StatusRequestTimeout,
					Message: "actor id: " + id.String(),
				},
			)
		default:
			return hz.RespondError(
				msg,
				&hz.Error{
					Status:  http.StatusInternalServerError,
					Message: "actor id: " + id.String() + ": " + err.Error(),
				},
			)
		}
	}
	// Forward the reply message onto the original caller.
	return msg.RespondMsg(runReply)
}

func (b *Broker) Stop() error {
	if b.subscription == nil {
		return nil
	}
	return b.subscription.Unsubscribe()
}
