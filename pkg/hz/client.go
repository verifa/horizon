package hz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	BucketObjects = "hz_objects"
	BucketMutex   = "hz_objects_mutex"
)

const (
	HeaderStatus        = "Hz-Status"
	HeaderAuthorization = "Hz-Authorization"
	HeaderFieldManager  = "Hz-Field-Manager"
)

const (
	CookieSession = "hz_session"
)

var (
	ErrNoRevision                = errors.New("no revision")
	ErrIncorrectRevision         = errors.New("incorrect revision")
	ErrNotFound                  = errors.New("not found")
	ErrApplyManagerRequired      = errors.New("apply: field manager required")
	ErrApplyObjectOrDataRequired = errors.New("apply: object or data required")
	ErrApplyObjectOrKeyRequired  = errors.New("apply: object or key required")

	ErrStoreNotResponding = errors.New("store not responding")

	ErrRunNoResponders         = errors.New("run: no brokers responding")
	ErrRunTimeout              = errors.New("run: broker timeout")
	ErrBrokerNoActorResponders = errors.New("broker: no actor responders")
	ErrBrokerActorTimeout      = errors.New("broker: actor timeout")
)

const (
	// format: HZ.api.broker.<kind>.<account>.<name>.<action>
	SubjectAPIBroker                  = "HZ.api.broker.*.*.*.*"
	SubjectAPIBrokerFmt               = "HZ.api.broker.%s.%s.%s.%s"
	SubjectInternalBroker             = "HZ.internal.broker.*.*.*.*"
	SubjectInternalBrokerFmt          = "HZ.internal.broker.%s.%s.%s.%s"
	SubjectInternalBrokerIndexKind    = 3
	SubjectInternalBrokerIndexAccount = 4
	SubjectInternalBrokerIndexName    = 5
	SubjectInternalBrokerIndexAction  = 6
	SubjectInternalBrokerLength       = 7

	// format: HZ.internal.actor.advertise.<kind>.<account>.<name>.<action>
	SubjectActorAdvertise    = "HZ.internal.actor.advertise.%s.*.*.%s"
	SubjectActorAdvertiseFmt = "HZ.internal.actor.advertise.%s.%s.%s.%s"
	// format:
	// HZ.internal.actor.run.<kind>.<account>.<name>.<action>.<actor_uuid>
	SubjectActorRun    = "HZ.internal.actor.run.%s.*.*.%s.%s"
	SubjectActorRunFmt = "HZ.internal.actor.run.%s.%s.%s.%s.%s"
)

const (
	SubjectStoreValidate = "store.validate.%s"
	SubjectStoreApply    = "store.apply.%s"
	SubjectStoreCreate   = "store.create.%s"
	SubjectStoreGet      = "store.get.%s"
	SubjectStoreList     = "store.list.%s"
)

type ObjectClient[T Objecter] struct {
	Client Client
}

func (oc ObjectClient[T]) Create(
	ctx context.Context,
	object T,
) error {
	data, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("marshalling object: %w", err)
	}
	return oc.Client.Create(ctx, KeyForObject(object), data)
}

func (oc ObjectClient[T]) Apply(
	ctx context.Context,
	object T,
	opts ...ApplyOption,
) error {
	opts = append(opts, WithApplyObject(object))
	return oc.Client.Apply(ctx, opts...)
}

func (oc ObjectClient[T]) Get(ctx context.Context, key string) (*T, error) {
	data, err := oc.Client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("getting object %q: %w", key, err)
	}
	var object T
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, fmt.Errorf("unmarshalling object: %w", err)
	}
	return &object, nil
}

func (oc ObjectClient[T]) List(
	ctx context.Context,
	opts ...ListOption,
) ([]*T, error) {
	opt := listOption{}
	for _, o := range opts {
		o(&opt)
	}

	if opt.key == "" {
		var t T
		key := KeyForObject(t)
		opts = append(opts, WithListKey(key))
	}
	data, err := oc.Client.List(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("listing objects: %w", err)
	}

	type Result struct {
		Data []*T `json:"data"`
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshalling objects: %w", err)
	}

	return result.Data, nil
}

// func (oc ObjectClient[T]) Delete(
// 	ctx context.Context,
// 	object T,
// ) error {
// 	// TODO: do not DELETE the object, but add the
// 	// metadata.deleteTimestamp field.
// 	// The controller then needs to handle deleting the object.
// 	// Make sure the NakWithDelay() is set to reocncile once the deleteTimestamp
// 	// has passed.
// 	// And once it has, and there are no finalizers or whatevever,
// 	// *then* delete the object.
// 	//
// 	// Then remove all the funky logic around the KV store for getting deleted
// 	// objects. Because once they are deleted in the KV store, they are deleted
// 	// in NCP. Current state is not that. Current state is deleted in KV
// 	// means "marked for deletion" in NCP, and deleteTimestamp will replace
// 	// this.
// 	kve, err := oc.Client.kv.Get(ctx, KeyForObject(object))
// 	if err != nil {
// 		return fmt.Errorf("get: %w", err)
// 	}
// 	// Prevent a double delete.
// 	if kve.Operation() == jetstream.KeyValueDelete {
// 		return nil
// 	}
// 	var t T
// 	if err := oc.Client.toObjectWithRevision(kve, &t); err != nil {
// 		return fmt.Errorf("unmarshal: %w", err)
// 	}
// 	// TODO: an object can/should be read only, so need to add this another way.
// 	t.ObjectDeleteAt(Time{Time: time.Now()})
// 	_, err = oc.Update(ctx, t)
// 	return err
// }

func (oc ObjectClient[T]) Validate(
	ctx context.Context,
	object T,
) error {
	bObject, err := json.Marshal(object)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	return oc.Client.Validate(ctx, object.ObjectKind(), bObject)
}

type RunOption func(*runOption)

func WithRunTimeout(timeout time.Duration) RunOption {
	return func(ro *runOption) {
		ro.timeout = timeout
	}
}

func WithRunLabelSelector(ls LabelSelector) RunOption {
	return func(ro *runOption) {
		ro.labelSelector = ls
	}
}

var runOptionDefault = runOption{
	timeout: time.Second * 5,
}

type runOption struct {
	timeout       time.Duration
	labelSelector LabelSelector
}

func (oc ObjectClient[T]) Run(
	ctx context.Context,
	actioner Actioner,
	object T,
	opts ...RunOption,
) (T, error) {
	ro := runOptionDefault
	for _, opt := range opts {
		opt(&ro)
	}
	var t T
	kind := t.ObjectKind()
	account := object.ObjectAccount()
	name := object.ObjectName()
	action := actioner.Action()

	subject := fmt.Sprintf(
		SubjectAPIBrokerFmt,
		kind,
		account,
		name,
		action,
	)
	var newObj T
	b, err := json.Marshal(object)
	if err != nil {
		return newObj, fmt.Errorf("marshalling request object: %w", err)
	}
	runMsg := RunMsg{
		Timeout:       ro.timeout,
		LabelSelector: ro.labelSelector,
		Data:          b,
	}
	bRunMsg, err := json.Marshal(runMsg)
	if err != nil {
		return newObj, fmt.Errorf("marshalling run message: %w", err)
	}
	slog.Info("run", "subject", subject)
	ctx, cancel := context.WithTimeout(ctx, ro.timeout)
	defer cancel()
	reply, err := oc.Client.Conn.RequestWithContext(ctx, subject, bRunMsg)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders):
			return newObj, ErrRunNoResponders
		case errors.Is(err, nats.ErrTimeout):
			return newObj, ErrRunTimeout
		default:
			return newObj, fmt.Errorf("request: %w", err)
		}
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return newObj, &Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("invalid status header: %s", err),
		}
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusServiceUnavailable:
			return newObj, ErrBrokerNoActorResponders
		case http.StatusRequestTimeout:
			return newObj, ErrBrokerActorTimeout
		default:
			return newObj, &Error{
				Status:  status,
				Message: string(reply.Data),
			}
		}
	}
	fmt.Println("")
	fmt.Println("")
	fmt.Println("reply.Data", string(reply.Data))
	fmt.Println("")
	fmt.Println("")
	fmt.Println("reply.Status", status)
	fmt.Println("")
	fmt.Println("")
	if err := json.Unmarshal(reply.Data, &newObj); err != nil {
		return newObj, fmt.Errorf("unmarshalling reply: %w", err)
	}
	return newObj, nil
}

// TODO: remove this after all... skip all these fluffy functions...
// Just create the client yourself.
func InternalClient(conn *nats.Conn) Client {
	return Client{
		Conn:     conn,
		Internal: true,
	}
}

func SessionFromRequest(req *http.Request) string {
	if sessionCookie, err := req.Cookie(CookieSession); err == nil {
		return sessionCookie.Value
	}
	return ""
}

type Client struct {
	Conn *nats.Conn

	// Internal is set to true to use the internal nats subjects.
	// This is used for service accounts (controllers) that have nats
	// credentials with permission to use the internal nats subjects.
	// For clients such as hzctl, this should be false causing the client to use
	// the `api` nats subjects (requiring authn/authz).
	Internal bool

	Session string
}

func (c Client) SubjectPrefix() string {
	if c.Internal {
		return "HZ.internal."
	}
	return "HZ.api."
}

func (c Client) Schema(
	ctx context.Context,
	kind string,
) (Schema, error) {
	subject := fmt.Sprintf("CTLR.schema.%s", kind)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestWithContext(ctx, subject, nil)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return Schema{}, errors.New("controller not responding")
		}
		return Schema{}, fmt.Errorf("request: %w", err)
	}

	var schema Schema
	if err := json.Unmarshal(reply.Data, &schema); err != nil {
		return Schema{}, fmt.Errorf(
			"unmarshal reply error: %w",
			err,
		)
	}

	return schema, nil
}

func (c Client) Validate(
	ctx context.Context,
	kind string,
	data []byte,
) error {
	subject := fmt.Sprintf(SubjectStoreValidate, kind)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return ErrStoreNotResponding
		}
		return fmt.Errorf("request: %w", err)
	}

	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return fmt.Errorf("invalid status header: %w", err)
	}
	return &Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

type ApplyOption func(*applyOptions)

type applyOptions struct {
	manager string
	object  Objecter
	data    []byte
	key     string
}

func WithApplyManager(manager string) ApplyOption {
	return func(ao *applyOptions) {
		ao.manager = manager
	}
}

func WithApplyObject(object Objecter) ApplyOption {
	return func(ao *applyOptions) {
		ao.object = object
	}
}

func WithApplyData(data []byte) ApplyOption {
	return func(ao *applyOptions) {
		ao.data = data
	}
}

func WithApplyKey(key string) ApplyOption {
	return func(ao *applyOptions) {
		ao.key = key
	}
}

func (c Client) Apply(
	ctx context.Context,
	opts ...ApplyOption,
) error {
	ao := applyOptions{}
	for _, opt := range opts {
		opt(&ao)
	}
	if ao.manager == "" {
		return ErrApplyManagerRequired
	}
	if ao.object == nil && ao.data == nil {
		return ErrApplyObjectOrDataRequired
	}
	if ao.object == nil && ao.key == "" {
		return ErrApplyObjectOrKeyRequired
	}
	if ao.object != nil {
		var err error
		ao.data, err = json.Marshal(ao.object)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
		if ao.key == "" {
			ao.key = KeyForObject(ao.object)
		}
	}

	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreApply, ao.key),
	)
	msg.Header.Set(HeaderFieldManager, ao.manager)
	msg.Header.Set(HeaderAuthorization, c.Session)
	msg.Data = ao.data
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return ErrStoreNotResponding
		}
		return fmt.Errorf("applying object: %w", err)
	}
	statusText := reply.Header.Get(HeaderStatus)
	status, err := strconv.Atoi(statusText)
	if err != nil {
		return fmt.Errorf("invalid status header %q: %w", statusText, err)
	}
	if status == http.StatusOK {
		return nil
	}
	return &Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func (c *Client) Create(
	ctx context.Context,
	key string,
	data []byte,
) error {
	msg := nats.NewMsg(c.SubjectPrefix() + fmt.Sprintf(SubjectStoreCreate, key))
	msg.Header.Set(HeaderAuthorization, c.Session)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestMsgWithContext(
		ctx,
		msg,
	)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return ErrStoreNotResponding
		}
		return fmt.Errorf("making request to store: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return fmt.Errorf("invalid status header: %w", err)
	}
	if status == http.StatusOK {
		return nil
	}
	return &Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func (c *Client) Get(
	ctx context.Context,
	key string,
) ([]byte, error) {
	msg := nats.NewMsg(c.SubjectPrefix() + fmt.Sprintf(SubjectStoreGet, key))
	msg.Header.Set(HeaderAuthorization, c.Session)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestMsgWithContext(
		ctx,
		msg,
	)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return nil, ErrStoreNotResponding
		}
		return nil, fmt.Errorf("making request to store: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return nil, fmt.Errorf("invalid status header: %w", err)
	}
	if status == http.StatusOK {
		return reply.Data, nil
	}
	return nil, &Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func WithListKey(key string) ListOption {
	return func(lo *listOption) {
		lo.key = key
	}
}

func WithListKeyFromObject(obj ObjectKeyer) ListOption {
	return func(lo *listOption) {
		lo.key = KeyForObject(obj)
	}
}

type ListOption func(*listOption)

type listOption struct {
	key string
}

func (c *Client) List(
	ctx context.Context,
	opts ...ListOption,
) ([]byte, error) {
	lo := listOption{}
	for _, opt := range opts {
		opt(&lo)
	}
	if lo.key == "" {
		return nil, fmt.Errorf("list: key required")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreList, lo.key),
	)
	msg.Header.Set(HeaderAuthorization, c.Session)
	reply, err := c.Conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return nil, ErrStoreNotResponding
		}
		return nil, fmt.Errorf("making request to store: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return nil, fmt.Errorf("invalid status header: %w", err)
	}
	if status == http.StatusOK {
		return reply.Data, nil
	}
	return nil, &Error{
		Status:  status,
		Message: string(reply.Data),
	}
}

func KeyForObject(obj ObjectKeyer) string {
	account := "*"
	if obj.ObjectAccount() != "" {
		account = obj.ObjectAccount()
	}
	name := "*"
	if obj.ObjectName() != "" {
		name = obj.ObjectName()
	}
	return KeyForObjectParams(
		obj.ObjectKind(),
		account,
		name,
	)
}

func KeyForObjectParams(
	kind string,
	account string,
	name string,
) string {
	return fmt.Sprintf(
		"%s.%s.%s",
		kind,
		account,
		name,
	)
}

// isErrWrongLastSequence returns true if the error is caused by a write
// operation to a stream with the wrong last sequence.
// For example, if a kv update with an outdated revision.
func isErrWrongLastSequence(err error) bool {
	var apiErr *jetstream.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence
	}
	return false
}

type RunMsg struct {
	Timeout       time.Duration   `json:"timeout,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	LabelSelector LabelSelector   `json:"labelSelector,omitempty"`
}

type AdvertiseMsg struct {
	LabelSelector LabelSelector `json:"labelSelector,omitempty"`
}
