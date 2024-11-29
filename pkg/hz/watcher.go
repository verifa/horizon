package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type WatcherOption func(*watcherOptions)

func WithWatcherFor(obj ObjectKeyer) WatcherOption {
	return func(o *watcherOptions) {
		o.forObject = obj
	}
}

// WithWatcherDurable sets the durable name for the nats consumer.
// In short, if you want to "load balance" the watcher across multiple
// instances, you can set the durable name to the same value for each.
// If you want each instance of the watcher to be completely independent,
// do not set the durable name of the consumer.
//
// Read more about consumers:
// https://docs.nats.io/nats-concepts/jetstream/consumers
func WithWatcherDurable(name string) WatcherOption {
	return func(o *watcherOptions) {
		o.durable = name
	}
}

func WithWatcherFn(
	fn func(event Event) (Result, error),
) WatcherOption {
	return func(o *watcherOptions) {
		o.fn = fn
	}
}

func WithWatcherCh(ch chan Event) WatcherOption {
	return func(o *watcherOptions) {
		o.ch = ch
	}
}

func WithWatcherFromNow() WatcherOption {
	return func(o *watcherOptions) {
		now := time.Now()
		o.startTime = &now
	}
}

type watcherOptions struct {
	forObject ObjectKeyer
	durable   string
	ackWait   time.Duration
	fn        func(event Event) (Result, error)
	ch        chan Event
	backoff   time.Duration
	startTime *time.Time
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
	if opt.fn == nil && opt.ch == nil {
		return fmt.Errorf("fn (callback) or ch (channel) is required")
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
	subject := "$KV." + kv.Bucket() + "." + KeyFromObject(opt.forObject)
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

	deliverPolicy := jetstream.DeliverLastPerSubjectPolicy
	if opt.startTime != nil {
		deliverPolicy = jetstream.DeliverByStartTimePolicy
	}
	con, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Description:    "Watcher for " + KeyFromObject(opt.forObject),
		AckPolicy:      jetstream.AckExplicitPolicy,
		DeliverPolicy:  deliverPolicy,
		OptStartTime:   opt.startTime,
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
			var result Result
			var err error
			if opt.ch != nil {
				event.Reply = make(chan EventResult)
				opt.ch <- event
				select {
				case eventResult := <-event.Reply:
					result = eventResult.Result
					err = eventResult.Err
				case <-time.After(time.Second * 5):
					slog.Error(
						"waiting for event reply",
						"event_operation",
						event.Operation,
						"key",
						event.Key,
					)
					_ = msg.NakWithDelay(opt.backoff)
					return
				}
			}
			if opt.fn != nil {
				result, err = opt.fn(event)
			}
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
				return
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
		rawKey := keyFromMsgSubject(kv, msg)
		key, err := ObjectKeyFromString(rawKey)
		if err != nil {
			slog.Error(
				"parsing key from subject",
				"error",
				err,
				"subject",
				msg.Subject(),
			)
			_ = msg.Term()
			return
		}
		if kvop == jetstream.KeyValueDelete {
			// If the operation is a KV delete, then the value has been
			// deleted from the key value store.
			// For watcher, this is the purge operation.
			event := Event{
				Operation: EventOperationPurge,
				Key:       key,
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
				Key:       key,
				Data:      msg.Data(),
			}
			handleEvent(msg, event)
			return
		}
		event := Event{
			Operation: EventOperationPut,
			Key:       key,
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
	Key       ObjectKeyer
	Reply     chan EventResult
}

func (e Event) Respond(result EventResult) error {
	if e.Reply == nil {
		return errors.New("no reply channel")
	}
	e.Reply <- result
	return nil
}

type EventResult struct {
	Result Result
	Err    error
}

type EventOperation string

const (
	// EventOperationPut indicates that the object has been created or updated.
	EventOperationPut EventOperation = "put"
	// EventOperationDelete indicates that the object has been marked for
	// deletion by setting the metadata.deletionTimestamp field.
	// It does not mean that the deleteionTimestamp has been reached yet,
	// so the deletionTimestamp may be in the future.
	EventOperationDelete EventOperation = "delete"
	// EventOperationPurge indicates that the object no longer exists in the kv
	// store.
	EventOperationPurge EventOperation = "purge"
)

// keyFromMsgSubject takes the subject for a msg and converts it to the
// corresponding key for a kv store.
//
// Under the hood, a nats kv store creates a stream.
// The subjects for messages on that stream contain a prefix.
// If we remove the prefix, we get the key which can be used to access values
// (messages) from the kv store.
func keyFromMsgSubject(kv jetstream.KeyValue, msg jetstream.Msg) string {
	key := strings.TrimPrefix(
		msg.Subject(),
		fmt.Sprintf("$KV.%s.", kv.Bucket()),
	)
	return key
}

func opFromMsg(msg jetstream.Msg) jetstream.KeyValueOp {
	kvop := jetstream.KeyValuePut
	if len(msg.Headers()) > 0 {
		op := msg.Headers().Get("KV-Operation")
		switch op {
		case "DEL":
			kvop = jetstream.KeyValueDelete
		case "PURGE":
			kvop = jetstream.KeyValuePurge
		default:
			kvop = jetstream.KeyValuePut
		}
	}
	return kvop
}
