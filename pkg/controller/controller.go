package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/verifa/horizon/pkg/extensions/core"
	"github.com/verifa/horizon/pkg/hz"
	"github.com/verifa/horizon/pkg/internal/openapiv3"
)

type Option func(*controllerOption)

func WithBucket(bucketObjects string) Option {
	return func(ro *controllerOption) {
		ro.bucketObjects = bucketObjects
	}
}

func WithReconciler(reconciler hz.Reconciler) Option {
	return func(ro *controllerOption) {
		ro.reconciler = reconciler
	}
}

func WithValidator(validator hz.Validator) Option {
	return func(ro *controllerOption) {
		ro.validators = append(ro.validators, validator)
	}
}

func WithValidatorCUE(b bool) Option {
	return func(ro *controllerOption) {
		ro.cueValidator = b
	}
}

// WithFor sets the object for which the controller is running for.
// A controller can only reconcile one object.
func WithFor(obj hz.Objecter) Option {
	return func(ro *controllerOption) {
		ro.forObject = obj
	}
}

// WithOwns sets objects that should be watched and checked if they are owned by
// the object given with [WithFor].
func WithOwns(obj hz.Objecter) Option {
	return func(ro *controllerOption) {
		ro.reconOwns = append(ro.reconOwns, obj)
	}
}

func WithStopTimeout(d time.Duration) Option {
	return func(ro *controllerOption) {
		ro.stopTimeout = d
	}
}

type controllerOption struct {
	bucketObjects string
	bucketMutex   string

	reconciler   hz.Reconciler
	validators   []hz.Validator
	cueValidator bool
	// validatorForceNone bool

	forObject hz.Objecter
	reconOwns []hz.Objecter

	stopTimeout time.Duration
}

func Start(
	ctx context.Context,
	nc *nats.Conn,
	opts ...Option,
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

	wg          sync.WaitGroup
	stopTimeout time.Duration

	subscriptions   []*nats.Subscription
	consumeContexts []jetstream.ConsumeContext
}

func (c *Controller) Start(
	ctx context.Context,
	opts ...Option,
) error {
	ro := controllerOption{
		bucketObjects: hz.BucketObjects,
		bucketMutex:   hz.BucketMutex,
		cueValidator:  true,
		stopTimeout:   time.Minute * 10,
	}
	for _, opt := range opts {
		opt(&ro)
	}
	if ro.forObject == nil {
		return fmt.Errorf("no object provided to controller")
	}

	c.stopTimeout = ro.stopTimeout
	if ro.bucketMutex == "" {
		ro.bucketMutex = ro.bucketObjects + "_mutex"
	}
	if ro.cueValidator {
		// Add the cue validator for the object.
		cueValidator := &hz.ValidateCUE{
			Object: ro.forObject,
		}
		if err := cueValidator.ParseObject(); err != nil {
			return fmt.Errorf("parsing object: %w", err)
		}
		// Make sure the default validator comes first.
		ro.validators = append([]hz.Validator{cueValidator}, ro.validators...)
	}
	// if err := c.startSchema(ctx, ro); err != nil {
	// 	return fmt.Errorf("start schema: %w", err)
	// }
	if err := c.applyCustomResourceDefinition(ctx, ro); err != nil {
		return fmt.Errorf("apply custom resource definition: %w", err)
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

func (c *Controller) applyCustomResourceDefinition(
	ctx context.Context,
	opt controllerOption,
) error {
	var (
		schema *openapiv3.Schema
		err    error
	)
	if oapiv3, ok := opt.forObject.(hz.ObjectOpenAPIV3Schemer); ok {
		schema, err = oapiv3.OpenAPIV3Schema()
	} else {
		schema, err = OpenAPIV3SchemaFromObject(opt.forObject)
	}
	if err != nil {
		return fmt.Errorf("getting object schema: %w", err)
	}

	name := fmt.Sprintf(
		"%s-%s-%s",
		opt.forObject.ObjectGroup(),
		opt.forObject.ObjectVersion(),
		opt.forObject.ObjectKind(),
	)
	singularName := strings.ToLower(opt.forObject.ObjectKind())
	crd := core.CustomResourceDefinition{
		ObjectMeta: hz.ObjectMeta{
			Name:      name,
			Namespace: hz.NamespaceRoot,
		},
		Spec: &core.CustomResourceDefinitionSpec{
			Group:   hz.P(opt.forObject.ObjectGroup()),
			Version: hz.P(opt.forObject.ObjectVersion()),
			Names: &core.CustomResourceDefinitionNames{
				Kind:     hz.P(opt.forObject.ObjectKind()),
				Singular: &singularName,
			},
			Schema: &core.CustomResourceDefinitionSchema{
				OpenAPIV3Schema: schema,
			},
		},
	}

	client := hz.NewClient(c.Conn, hz.WithClientInternal(true))
	_, err = client.Apply(ctx, hz.WithApplyObject(crd))
	if err != nil {
		return fmt.Errorf("applying custom resource definition: %w", err)
	}

	return nil
}

func (c *Controller) startSchema(
	_ context.Context,
	opt controllerOption,
) error {
	obj := opt.forObject
	var (
		schema *openapiv3.Schema
		err    error
	)

	if oapiv3, ok := opt.forObject.(hz.ObjectOpenAPIV3Schemer); ok {
		schema, err = oapiv3.OpenAPIV3Schema()
	} else {
		schema, err = OpenAPIV3SchemaFromObject(obj)
	}
	if err != nil {
		return fmt.Errorf("getting object schema: %w", err)
	}
	bSchema, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("marshalling schema: %w", err)
	}
	subject := fmt.Sprintf(
		hz.SubjectCtlrSchema,
		obj.ObjectGroup(),
		obj.ObjectVersion(),
		obj.ObjectKind(),
	)
	sub, err := c.Conn.QueueSubscribe(subject, "schema", func(msg *nats.Msg) {
		go func() {
			_ = msg.Respond(bSchema)
		}()
	})
	if err != nil {
		return fmt.Errorf("subscribing validator: %w", err)
	}
	c.subscriptions = append(c.subscriptions, sub)
	return nil
}

// startValidators subscribes to the validator subjects and validates objects as
// they come in.
func (c *Controller) startValidators(
	ctx context.Context,
	opt controllerOption,
) error {
	obj := opt.forObject
	{
		subject := fmt.Sprintf(
			hz.SubjectCtlrValidateCreate,
			obj.ObjectGroup(),
			obj.ObjectVersion(),
			obj.ObjectKind(),
		)
		sub, err := c.Conn.QueueSubscribe(
			subject,
			"validate-create",
			func(msg *nats.Msg) {
				go c.handleValidateCreate(ctx, opt, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing validator %q: %w", subject, err)
		}
		c.subscriptions = append(c.subscriptions, sub)
	}
	{
		subject := fmt.Sprintf(
			hz.SubjectCtlrValidateUpdate,
			obj.ObjectGroup(),
			obj.ObjectVersion(),
			obj.ObjectKind(),
		)
		sub, err := c.Conn.QueueSubscribe(
			subject,
			"validate-update",
			func(msg *nats.Msg) {
				go c.handleValidateUpdate(ctx, opt, msg)
			},
		)
		if err != nil {
			return fmt.Errorf("subscribing validator %q: %w", subject, err)
		}
		c.subscriptions = append(c.subscriptions, sub)
	}
	return nil
}

func (c *Controller) handleValidateCreate(
	ctx context.Context,
	opt controllerOption,
	msg *nats.Msg,
) {
	var vErr *hz.Error
	for _, validator := range opt.validators {
		if err := validator.ValidateCreate(ctx, msg.Data); err != nil {
			vErr = &hz.Error{
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			}
			slog.Info("validate create error", "error", err)
			break
		}
	}
	if vErr != nil {
		_ = hz.RespondError(msg, vErr)
		return
	}
	_ = hz.RespondOK(msg, nil)
}

func (c *Controller) handleValidateUpdate(
	ctx context.Context,
	opt controllerOption,
	msg *nats.Msg,
) {
	var metaObj hz.MetaOnlyObject
	if err := json.Unmarshal(msg.Data, &metaObj); err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status: http.StatusBadRequest,
			Message: fmt.Sprintf(
				"unmarshalling object: %s",
				err.Error(),
			),
		})
		return
	}
	// Need to fetch the existing object and pass it to the validators.
	client := hz.NewClient(c.Conn, hz.WithClientInternal(true))
	old, err := client.Get(ctx, hz.WithGetKey(metaObj))
	if err != nil {
		_ = hz.RespondError(msg, &hz.Error{
			Status: http.StatusInternalServerError,
			Message: fmt.Sprintf(
				"getting existing object from store: %s",
				err.Error(),
			),
		})
		return
	}
	var vErr *hz.Error
	for _, validator := range opt.validators {
		if err := validator.ValidateUpdate(ctx, old, msg.Data); err != nil {
			vErr = &hz.Error{
				Status:  http.StatusBadRequest,
				Message: err.Error(),
			}
			slog.Info("validate update error", "error", err)
			break
		}
	}
	if vErr != nil {
		_ = hz.RespondError(msg, vErr)
		return
	}
	_ = hz.RespondOK(msg, nil)
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
	subject := "$KV." + kv.Bucket() + "." + hz.KeyFromObject(forObj)
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
		kvop := opFromMsg(msg)
		if kvop == jetstream.KeyValueDelete {
			// If the operation is a KV delete, then the value has been
			// deleted, so ack it.
			// This is different from what horizon considers a delete operation.
			// In horizon, a delete operation sets the
			// metadata.deletionTimestamp. In the kv store, a delete operation
			// means the whole object is gone (i.e. what horizon's considers
			// a purge).
			_ = msg.Ack()
			return
		}
		key := keyFromMsgSubject(kv, msg)
		go c.handleControlLoop(
			ctx,
			opt.reconciler,
			kv,
			mutex,
			key,
			msg,
			ttl,
		)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	c.consumeContexts = append(c.consumeContexts, cc)

	for _, obj := range opt.reconOwns {
		subject := "$KV." + kv.Bucket() + "." + hz.KeyFromObject(obj)
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
			var emptyObject hz.MetaOnlyObject
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
			key := hz.KeyFromObject(hz.ObjectKey{
				Group:     ownerRef.Group,
				Kind:      ownerRef.Kind,
				Name:      ownerRef.Name,
				Namespace: ownerRef.Namespace,
			})
			go c.handleControlLoop(
				ctx,
				opt.reconciler,
				kv,
				mutex,
				key,
				msg,
				ttl,
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

	// Wait for all reconcile loops to finish, with a timeout.
	if c.stopWaitTimeout() {
		errs = errors.Join(
			errs,
			fmt.Errorf(
				"timeout after %s waiting for reconcile loops to finish",
				c.stopTimeout,
			),
		)
	}
	return errs
}

func (c *Controller) stopWaitTimeout() bool {
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.wg.Wait()
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
				c.stopTimeout,
			)
		case <-done:
			return false // completed normally
		case <-time.After(c.stopTimeout):
			return true // timed out
		}
	}
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
	reconciler hz.Reconciler,
	kv jetstream.KeyValue,
	mutex mutex,
	key string,
	msg jetstream.Msg,
	ttl time.Duration,
) {
	c.wg.Add(1)
	defer c.wg.Done()
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
	objKey, err := hz.ObjectKeyFromString(key)
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
	req := hz.Request{
		Key: objKey,
	}
	var (
		reconcileResult hz.Result
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
	slog.Info("reconciling object", "key", key)
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
				slog.Info("ticker in progress")
				if err := lock.InProgress(); err != nil {
					slog.Error("resetting mutex lock", "error", err)
				}
				if err := msg.InProgress(); err != nil {
					slog.Error("marking  message in progress", "error", err)
				}
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
	default:
		slog.Error("unknown reconcile result", "result", reconcileResult)
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
