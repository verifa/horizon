package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type WatcherOption func(*watcherOptions)

func WithWatcherForObject(obj ObjectKeyer) WatcherOption {
	return func(o *watcherOptions) {
		o.forObject = obj
	}
}

func WithWatcherFn(
	fn func(event Event) (Result, error),
) WatcherOption {
	return func(o *watcherOptions) {
		o.fn = fn
	}
}

type watcherOptions struct {
	forObject ObjectKeyer
	ackWait   time.Duration
	fn        func(event Event) (Result, error)
	backoff   time.Duration
}

var defaultWatcherOptions = watcherOptions{
	ackWait: 5 * time.Second,
	backoff: time.Second,
}

type Watcher struct {
	Conn *nats.Conn

	consumeContext jetstream.ConsumeContext
	isInit         bool
	Init           chan struct{}
}

func StartWatcher(
	ctx context.Context,
	conn *nats.Conn,
	opts ...WatcherOption,
) (*Watcher, error) {
	w := &Watcher{Conn: conn}
	if err := w.Start(ctx, opts...); err != nil {
		return nil, fmt.Errorf("starting watcher: %w", err)
	}
	return w, nil
}

func (w *Watcher) Close() {
	w.consumeContext.Stop()
}

func (w *Watcher) Start(ctx context.Context, opts ...WatcherOption) error {
	w.Init = make(chan struct{})
	if w.consumeContext != nil {
		return fmt.Errorf("watcher already started")
	}
	opt := defaultWatcherOptions
	for _, o := range opts {
		o(&opt)
	}

	if opt.forObject == nil {
		return fmt.Errorf("for object is required")
	}
	if opt.fn == nil {
		return fmt.Errorf("fn (callback) is required")
	}
	js, err := jetstream.New(w.Conn)
	if err != nil {
		return fmt.Errorf("new jetstream: %w", err)
	}
	kv, err := js.KeyValue(ctx, BucketObjects)
	if err != nil {
		return fmt.Errorf(
			"conntecting to objects kv bucket %q: %w",
			BucketObjects,
			err,
		)
	}
	stream, err := js.Stream(ctx, "KV_"+kv.Bucket())
	if err != nil {
		return fmt.Errorf("get stream %q: %w", "KV_"+kv.Bucket(), err)
	}
	subject := "$KV." + kv.Bucket() + "." + KeyForObject(opt.forObject)
	// Get the last message for the subject because we want the message
	// sequence.
	// As we consume messages we can compare the message sequence with the
	// latest message to find out when we have "caught up" with the stream.
	// If no last message exists (i.e. there is no message for the subject) then
	// there is nothing to catch up with.
	lastMsg, err := stream.GetLastMsgForSubject(ctx, subject)
	if err != nil {
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			return fmt.Errorf("get last msg for subject: %w", err)
		}
		w.isInit = true
		close(w.Init)
	}
	con, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Description:    "Watcher for " + opt.forObject.ObjectKind(),
		AckPolicy:      jetstream.AckExplicitPolicy,
		DeliverPolicy:  jetstream.DeliverLastPerSubjectPolicy,
		FilterSubjects: []string{subject},
		MaxAckPending:  -1,
		// AckWait specifies how long a consumer waits before considering a
		// message delivered to a consumer as lost.
		// Hence, the consumer needs to ack/nak or mark the msg as in progress
		// before this time expires.
		AckWait: opt.ackWait,
	})
	if err != nil {
		return fmt.Errorf("create for consumer: %w", err)
	}
	cc, err := con.Consume(func(msg jetstream.Msg) {
		msgMeta, err := msg.Metadata()
		if err != nil {
			slog.Error(
				"getting msg metadata",
				"subject",
				msg.Subject(),
				"error",
				err,
			)
			_ = msg.Term()
		}
		kvop := opFromMsg(msg)
		handleEvent := func(msg jetstream.Msg, event Event) {
			result, err := opt.fn(event)
			if err != nil {
				slog.Error(
					"handling event",
					"error",
					err,
					"backoff",
					opt.backoff,
					"event_operation",
					event.Operation,
				)
				_ = msg.NakWithDelay(opt.backoff)
			}
			switch {
			case result.IsZero():
				if !w.isInit &&
					msgMeta.Sequence.Stream == lastMsg.Sequence {
					close(w.Init)
				}
				_ = msg.Ack()
			case result.Requeue:
				_ = msg.Nak()
			case result.RequeueAfter > 0:
				_ = msg.NakWithDelay(result.RequeueAfter)
			}
		}
		if kvop == jetstream.KeyValueDelete {
			// If the operation is a KV delete, then the value has been
			// deleted from the key value store.
			// For watcher, this is the purge operation.
			event := Event{
				Operation: EventOperationPurge,
				Key:       keyFromMsgSubject(kv, msg),
				Data:      nil,
			}
			handleEvent(msg, event)
			return
		}
		var gObj GenericObject
		if err := json.Unmarshal(msg.Data(), &gObj); err != nil {
			slog.Error(
				"unmarshalling object",
				"error",
				err,
				"data",
				string(msg.Data()),
			)
			_ = msg.Term()
			return
		}
		// Check if the object is marked for deletion.
		if gObj.DeletionTimestamp != nil {
			event := Event{
				Operation: EventOperationDelete,
				Key:       keyFromMsgSubject(kv, msg),
				Data:      msg.Data(),
			}
			handleEvent(msg, event)
			return
		}
		event := Event{
			Operation: EventOperationPut,
			Key:       keyFromMsgSubject(kv, msg),
			Data:      msg.Data(),
		}
		handleEvent(msg, event)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	w.consumeContext = cc
	return nil
}

func (w *Watcher) WaitUntilInit() {
	<-w.Init
}

type Event struct {
	Operation EventOperation
	Data      []byte
	Key       string
}

type EventOperation int

const (
	EventOperationPut EventOperation = iota
	EventOperationDelete
	EventOperationPurge
)
