package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type ControllerOption func(*controllerOption)

func WithControllerBucket(bucketObjects string) ControllerOption {
	return func(ro *controllerOption) {
		ro.bucketObjects = bucketObjects
	}
}

func WithControllerReconciler(reconciler Reconciler) ControllerOption {
	return func(ro *controllerOption) {
		ro.reconciler = reconciler
	}
}

func WithControllerValidator(validator Validator) ControllerOption {
	return func(ro *controllerOption) {
		ro.validators = append(ro.validators, validator)
	}
}

func WithControllerValidatorCUE(b bool) ControllerOption {
	return func(ro *controllerOption) {
		ro.cueValidator = b
	}
}

// WithControllerValidatorForceNone forces the controller to accept no
// validators. It is intended for testing purposes. It is highly recommended to
// use a validator to ensure data quality.
func WithControllerValidatorForceNone() ControllerOption {
	return func(ro *controllerOption) {
		ro.validatorForceNone = true
	}
}

func WithControllerFor(obj Objecter) ControllerOption {
	return func(ro *controllerOption) {
		ro.forObject = obj
	}
}

func WithControllerOwns(obj Objecter) ControllerOption {
	return func(ro *controllerOption) {
		ro.reconOwns = append(ro.reconOwns, obj)
	}
}

type controllerOption struct {
	bucketObjects string
	bucketMutex   string

	reconciler         Reconciler
	validators         []Validator
	cueValidator       bool
	validatorForceNone bool

	forObject Objecter
	reconOwns []Objecter
}

var controllerOptionsDefault = controllerOption{
	bucketObjects: BucketObjects,
	bucketMutex:   BucketMutex,
	cueValidator:  true,
}

func StartController(
	ctx context.Context,
	nc *nats.Conn,
	opts ...ControllerOption,
) (*Controller, error) {
	ctlr := Controller{
		Conn: nc,
	}
	if err := ctlr.Start(ctx, opts...); err != nil {
		return nil, fmt.Errorf("start: %w", err)
	}

	return &ctlr, nil
}

type Controller struct {
	Conn *nats.Conn

	subscriptions   []*nats.Subscription
	consumeContexts []jetstream.ConsumeContext
}

func (c *Controller) Start(
	ctx context.Context,
	opts ...ControllerOption,
) error {
	ro := controllerOptionsDefault
	for _, opt := range opts {
		opt(&ro)
	}
	if ro.forObject == nil {
		return fmt.Errorf("no object provided to controller")
	}
	// Check the forObject value is not a pointer, as this causes problems for
	// the cue encoder. If it is a pointer, get its element.
	if reflect.ValueOf(ro.forObject).Type().Kind() == reflect.Ptr {
		var ok bool
		ro.forObject, ok = reflect.ValueOf(ro.forObject).
			Elem().
			Interface().(Objecter)
		if !ok {
			return fmt.Errorf("getting element from object pointer")
		}
	}
	if ro.bucketMutex == "" {
		ro.bucketMutex = ro.bucketObjects + "_mutex"
	}
	if ro.cueValidator {
		// Add the cue validator for the object.
		cueValidator := &CUEValidator{
			Object: ro.forObject,
		}
		if err := cueValidator.ParseObject(); err != nil {
			return fmt.Errorf("parsing object: %w", err)
		}
		// Make sure the default validator comes first.
		ro.validators = append([]Validator{cueValidator}, ro.validators...)
	}
	if err := c.startSchema(ctx, ro); err != nil {
		return fmt.Errorf("start schema: %w", err)
	}

	if err := c.startValidators(ctx, ro); err != nil {
		return fmt.Errorf("start validator: %w", err)
	}
	if ro.reconciler != nil {
		if err := c.startReconciler(ctx, ro); err != nil {
			return fmt.Errorf("start reconciler: %w", err)
		}
	}
	return nil
}

func (c *Controller) startSchema(
	_ context.Context,
	opt controllerOption,
) error {
	obj := opt.forObject
	objSpec, err := OpenAPISpecFromObject(obj)
	if err != nil {
		return fmt.Errorf("getting object spec: %w", err)
	}
	schema, err := objSpec.Schema()
	if err != nil {
		return fmt.Errorf("getting schema: %w", err)
	}
	bSchema, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshalling schema: %w", err)
	}
	subject := fmt.Sprintf(
		"CTLR.schema.%s.%s",
		obj.ObjectGroup(),
		obj.ObjectKind(),
	)
	sub, err := c.Conn.QueueSubscribe(subject, "schema", func(msg *nats.Msg) {
		_ = msg.Respond(bSchema)
	})
	if err != nil {
		return fmt.Errorf("subscribing validator: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)
	return nil
}

func (c *Controller) startValidators(
	ctx context.Context,
	opt controllerOption,
) error {
	obj := opt.forObject
	subject := fmt.Sprintf(
		"CTLR.validate.%s.%s",
		obj.ObjectGroup(),
		obj.ObjectKind(),
	)
	sub, err := c.Conn.QueueSubscribe(
		subject,
		"validator",
		func(msg *nats.Msg) {
			var vErr *Error
			for _, validator := range opt.validators {
				slog.Info("validate", "subject", msg.Subject)
				if err := validator.Validate(ctx, msg.Data); err != nil {
					vErr = &Error{
						Status:  http.StatusBadRequest,
						Message: err.Error(),
					}
					slog.Info("validate error", "error", err)
					// Break once the first validator finishes.
					// Reason: custom validators can depend on something like
					// the default CUE validator and don't need to re-validate
					// all the basic values before doing something
					// more advanced.
					break
				}
			}
			if vErr != nil {
				_ = RespondError(msg, vErr)
				return
			}
			_ = msg.Respond(nil)
		},
	)
	if err != nil {
		return fmt.Errorf("subscribing validator: %w", err)
	}
	slog.Info("controller validator subscribed", "subject", subject)
	c.subscriptions = append(c.subscriptions, sub)
	return nil
}

func (c *Controller) startReconciler(
	ctx context.Context,
	opt controllerOption,
) error {
	js, err := jetstream.New(c.Conn)
	if err != nil {
		return fmt.Errorf("jetstream: %w", err)
	}
	kv, err := js.KeyValue(ctx, opt.bucketObjects)
	if err != nil {
		return fmt.Errorf(
			"getting keyvalue bucket %q: %w",
			opt.bucketObjects,
			err,
		)
	}
	mutex, err := MutexFromBucket(ctx, js, opt.bucketMutex)
	if err != nil {
		return fmt.Errorf("obtaining mutex: %w", err)
	}

	ttl := mutex.ttl

	forObj := opt.forObject
	stream, err := js.Stream(ctx, "KV_"+kv.Bucket())
	if err != nil {
		return fmt.Errorf("stream: %w", err)
	}
	subject := "$KV." + kv.Bucket() + "." + KeyFromObject(forObj)
	con, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:           "rc_" + forObj.ObjectKind(),
		Durable:        "rc_" + forObj.ObjectKind(),
		Description:    "Reconciler for " + forObj.ObjectKind(),
		AckPolicy:      jetstream.AckExplicitPolicy,
		DeliverPolicy:  jetstream.DeliverLastPerSubjectPolicy,
		FilterSubjects: []string{subject},
		MaxAckPending:  -1,
		// AckWait specifies how long a consumer waits before considering a
		// message delivered to a consumer as lost.
		// Hence, the consumer needs to ack/nak or mark the msg as in progress
		// before this time expires.
		AckWait: ttl,
		// MaxAckPendingPerSubject would allow only one concurrent consume loop
		// *per consumer*, which still does not solve everything.
		// We need one concurrent consume loop per object, including reconcile
		// loops triggered by ownership.
		// Issue: https://github.com/nats-io/nats-server/issues/4273
	})
	if err != nil {
		return fmt.Errorf("create for consumer: %w", err)
	}
	cc, err := con.Consume(func(msg jetstream.Msg) {
		slog.Info("consumer", "subject", msg.Subject())
		kvop := opFromMsg(msg)
		if kvop == jetstream.KeyValueDelete {
			// If the operation is a KV delete, then the value has been
			// deleted, so ack it.
			_ = msg.Ack()
			return
		}
		key := keyFromMsgSubject(kv, msg)
		isOwnerReconcile := false
		go c.handleControlLoop(
			ctx,
			opt.reconciler,
			kv,
			mutex,
			key,
			msg,
			ttl,
			isOwnerReconcile,
		)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	c.consumeContexts = append(c.consumeContexts, cc)

	for _, obj := range opt.reconOwns {
		subject := "$KV." + kv.Bucket() + "." + KeyFromObject(obj)
		con, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
			Name:           "rc_" + forObj.ObjectKind() + "_o_" + obj.ObjectKind(),
			Description:    "Reconciler for " + forObj.ObjectKind() + " owns " + obj.ObjectKind(),
			DeliverPolicy:  jetstream.DeliverLastPerSubjectPolicy,
			FilterSubjects: []string{subject},
			MaxAckPending:  -1,
			// AckWait specifies how long a consumer waits before considering a
			// message delivered to a consumer as lost.
			// Hence, the consumer needs to ack/nak or mark the msg as in
			// progress before this time expires.
			AckWait: ttl,
		})
		if err != nil {
			return fmt.Errorf("create owns consumer: %w", err)
		}
		cc, err := con.Consume(func(msg jetstream.Msg) {
			kvop := opFromMsg(msg)
			if kvop == jetstream.KeyValueDelete {
				// If the operation is a KV delete, then the value has been
				// deleted, so ack it.
				_ = msg.Ack()
				return
			}

			// This consumer is for the child objects of the parent object.
			// Hence, check if the child object (msg) is owned by the parent
			// for which the reconciler is running.
			var emptyObject EmptyObjectWithMeta
			if err := json.Unmarshal(msg.Data(), &emptyObject); err != nil {
				slog.Error("unmarshal msg to empty object", "error", err)
				_ = msg.Term()
				return
			}
			if len(emptyObject.OwnerReferences) == 0 {
				_ = msg.Ack()
				return
			}
			ownerRef, ok := emptyObject.ObjectOwnerReference(forObj)
			if !ok {
				_ = msg.Ack()
				return
			}
			// Key for the owner (parent) object.
			key := KeyFromObject(ObjectKey{
				Name:    ownerRef.Name,
				Account: ownerRef.Account,
				Group:   ownerRef.Group,
				Kind:    ownerRef.Kind,
			})
			isOwnerReconcile := true
			go c.handleControlLoop(
				ctx,
				opt.reconciler,
				kv,
				mutex,
				key,
				msg,
				ttl,
				isOwnerReconcile,
			)
		})
		if err != nil {
			return fmt.Errorf("consume: %w", err)
		}
		c.consumeContexts = append(c.consumeContexts, cc)
	}

	return nil
}

func (c *Controller) Stop() error {
	var errs error
	for _, cc := range c.consumeContexts {
		cc.Stop()
	}
	for _, sub := range c.subscriptions {
		if err := sub.Unsubscribe(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

// handleControlLoop is the main control loop for the controller.
// - kv is the kv store that the controller is watching
// - mutex is the mutex bucket for the kv store
// - key is the key for the object we are reconciling
// - msg is the message that triggered the control loop
//
// NOTE: the key is not always derived from the msg.
// Consider the case of ownership. If a parent owns a child, the message may be
// for the child which triggers to reconcile, but the key should point to the
// owner object of the child (parent).
func (c *Controller) handleControlLoop(
	ctx context.Context,
	reconciler Reconciler,
	kv jetstream.KeyValue,
	mutex mutex,
	key string,
	msg jetstream.Msg,
	ttl time.Duration,
	isOwnerReconcile bool,
) {
	slog.Info("control loop", "key", key)
	// Check that the message is the last message for the subject.
	// If not, we don't care about it and want to avoid acquiring the lock.
	isLast, err := isLastMsg(ctx, kv, msg)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			// Could be that the key was deleted, which is fine.
			// Ack the message and return.
			_ = msg.Ack()
			return
		}
		slog.Error("verifying last message for subject", "error", err)
		_ = msg.NakWithDelay(time.Second)
		return
	}
	// If message is not the last message, we don't care about it.
	// Ack the message and return.
	if !isLast {
		_ = msg.Ack()
		return
	}
	// Get the object key from the nats subject / kv key.
	objKey, err := objectKeyFromKey(key)
	if err != nil {
		slog.Error("getting object key from key", "key", key, "error", err)
		_ = msg.NakWithDelay(time.Second)
		return
	}
	// Acquire lock from the mutex.
	lock, err := mutex.Lock(ctx, key)
	if err != nil {
		if errors.Is(err, ErrKeyLocked) {
			// Someone else has the lock, which is fine.
			// Set some reconcile time and finish gracefully.
			// The control loop should start and wait for the lock again.
			_ = msg.NakWithDelay(time.Second)
			return
		}
		slog.Error("acquiring lock", "key", key, "error", err)
		// Try again, but not immediately.
		_ = msg.NakWithDelay(time.Second)
		return
	}
	defer func() {
		// If releasing the lock fails, the lock will be released automatically
		// when the TTL for the mutex bucket expires.
		if err := lock.Release(); err != nil {
			slog.Error("unlocking", "lock", lock, "error", err)
		}
	}()

	// Prepare the request and call the reconciler.
	req := Request{
		Key: objKey,
	}
	var (
		reconcileResult Result
		reconcileErr    error
		reconcileDone   = make(chan struct{})
	)
	reconcile := func() {
		// Close the channel when the reconciler is done.
		defer func() {
			close(reconcileDone)
			// In case the reconciler panics, we want to recover and redeliver
			// the message within a timely manner.
			if err := recover(); err != nil {
				reconcileErr = fmt.Errorf("panic: %v: %s", err, debug.Stack())
			}
		}()
		// Create a context with a hard timeout.
		// This is the max time a reconciler can run for.
		hardTimeout := time.Hour
		ctx, cancel := context.WithTimeout(ctx, hardTimeout)
		defer cancel()
		reconcileResult, reconcileErr = reconciler.Reconcile(ctx, req)
	}
	go reconcile()

	// Setup an auto-ticker for the message, which keeps the message alive and
	// avoids the consumer AckWait or lock TTL expiring.
	inProgressTicker := func() {
		ticker := time.NewTicker(ttl / 2)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := lock.InProgress(); err != nil {
					slog.Error("marking message in progress", "error", err)
				}
				slog.Info("ticker in progress")
			case <-reconcileDone:
				return
			}
		}
	}
	inProgressTicker()
	if reconcileErr != nil {
		backoff, err := exponentialBackoff(msg)
		if err != nil {
			slog.Error("getting exponential backoff", "error", err)
			_ = msg.NakWithDelay(time.Second * 10)
			return
		}
		slog.Error(
			"reconcile",
			"subject",
			msg.Subject(),
			"key",
			req.Key,
			"backoff",
			backoff.String(),
			"error",
			reconcileErr,
		)
		_ = msg.NakWithDelay(backoff)
		return
	}

	switch {
	case reconcileResult.IsZero():
		// If the reconcile loop is for an owner reference, we do NOT
		// want to handle any cleanup logic.
		// We leave this to the object's own reconcile loop.
		if !isOwnerReconcile {
			// Check if the object is being deleted, because then we want to
			// actually delete it in the KV now.
			var eo EmptyObjectWithMeta
			if err := json.Unmarshal(msg.Data(), &eo); err != nil {
				slog.Error("unmarshal msg to empty object", "error", err)
				_ = msg.NakWithDelay(time.Second)
				return
			}
			if eo.ObjectMeta.DeletionTimestamp != nil &&
				eo.ObjectMeta.DeletionTimestamp.Before(time.Now()) {
				slog.Info("control loop delete", "key", req.Key)
				// Delete the object and ack the message.
				if err := kv.Delete(ctx, KeyFromObject(req.Key)); err != nil {
					slog.Error("deleting object", "error", err)
					_ = msg.NakWithDelay(time.Second)
					return
				}
				_ = msg.Ack()
				return
			}
		}

		slog.Info("result zero", "key", req.Key)
		// TODO: make this configurable. Default is to ACK the message
		// so that it never reconciles. It reconciles again when the object
		// changes.
		if err := msg.Ack(); err != nil {
			slog.Error("result zero: ack", "error", err)
		}
	case reconcileResult.RequeueAfter > 0:
		if err := msg.NakWithDelay(reconcileResult.RequeueAfter); err != nil {
			slog.Error("result requeue after: nak with delay", "error", err)
		}
	case reconcileResult.Requeue:
		// If requeue is set, reconcile immediately.
		if err := msg.Nak(); err != nil {
			slog.Error("result requeue: nak", "error", err)
		}
	}
}

// isLastMsg checks if the sequence number of the given messages matches the
// revision (sequence) of the value in the key value store.
//
// A Get() operation for the kv store fetches the latest message for the key
// (subject) in the kv (stream).
func isLastMsg(
	ctx context.Context,
	kv jetstream.KeyValue,
	msg jetstream.Msg,
) (bool, error) {
	key := keyFromMsgSubject(kv, msg)
	kve, err := kv.Get(ctx, key)
	if err != nil {
		return false, fmt.Errorf("getting key value entry: %w", err)
	}
	meta, err := msg.Metadata()
	if err != nil {
		return false, fmt.Errorf("getting message metadata: %w", err)
	}
	if kve.Revision() == meta.Sequence.Stream {
		return true, nil
	}
	return false, nil
}

func exponentialBackoff(msg jetstream.Msg) (time.Duration, error) {
	meta, err := msg.Metadata()
	if err != nil {
		return 0, fmt.Errorf("getting message metadata: %w", err)
	}

	backoff := time.Second * 10
	if meta.NumDelivered == 0 {
		return backoff, nil
	}
	exp := math.Pow(2, float64(meta.NumDelivered))
	const secondsPerDay = 86400
	if exp > secondsPerDay {
		backoff = time.Second * secondsPerDay
	} else {
		backoff = time.Duration(exp * float64(time.Second))
	}
	return backoff, nil
}

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

func IgnoreNotFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
