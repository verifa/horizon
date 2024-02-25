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
	"github.com/tidwall/sjson"
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
	ErrNoRevision        = errors.New("no revision")
	ErrIncorrectRevision = errors.New("incorrect revision")
	ErrNotFound          = &Error{
		Status:  http.StatusNotFound,
		Message: "not found",
	}
	ErrApplyManagerRequired       = errors.New("apply: field manager required")
	ErrApplyObjectOrDataRequired  = errors.New("apply: object or data required")
	ErrApplyObjectOrKeyRequired   = errors.New("apply: object or key required")
	ErrCreateObjectOrDataRequired = errors.New(
		"create: object or data required",
	)
	ErrCreateObjectOrKeyRequired = errors.New("create: object or key required")

	ErrClientNoSession = errors.New("client: no session")

	ErrStoreNotResponding = errors.New("store: not responding")

	ErrRunNoResponders         = errors.New("run: no brokers responding")
	ErrRunTimeout              = errors.New("run: broker timeout")
	ErrBrokerNoActorResponders = errors.New("broker: no actor responders")
	ErrBrokerActorTimeout      = errors.New("broker: actor timeout")
)

const (
	SubjectAPIAllowAll = "HZ.api.>"

	// format: HZ.api.broker.<group><kind>.<account>.<name>.<action>
	SubjectAPIBroker                  = "HZ.api.broker.*.*.*.*.*"
	SubjectInternalBroker             = "HZ.internal.broker.*.*.*.*.*"
	SubjectInternalBrokerIndexGroup   = 3
	SubjectInternalBrokerIndexKind    = 4
	SubjectInternalBrokerIndexAccount = 5
	SubjectInternalBrokerIndexName    = 6
	SubjectInternalBrokerIndexAction  = 7
	SubjectInternalBrokerLength       = 8
	SubjectBrokeRun                   = "broker.%s.%s.%s.%s.%s"

	// format:
	// HZ.internal.actor.advertise.<group><kind>.<account>.<name>.<action>
	SubjectActorAdvertise    = "HZ.internal.actor.advertise.%s.%s.*.*.%s"
	SubjectActorAdvertiseFmt = "HZ.internal.actor.advertise.%s.%s.%s.%s.%s"
	// format:
	// HZ.internal.actor.run.<group>.<kind>.<account>.<name>.<action>.<actor_uuid>
	SubjectActorRun    = "HZ.internal.actor.run.%s.%s.*.*.%s.%s"
	SubjectActorRunFmt = "HZ.internal.actor.run.%s.%s.%s.%s.%s.%s"
)

const (
	SubjectStoreSchema   = "store.schema.%s.%s"
	SubjectStoreValidate = "store.validate.%s.%s"
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
	return oc.Client.Create(ctx, WithCreateObject(object))
}

func (oc ObjectClient[T]) Apply(
	ctx context.Context,
	object T,
	opts ...ApplyOption,
) error {
	opts = append(opts, WithApplyObject(object))
	return oc.Client.Apply(ctx, opts...)
}

type GetOption func(*getOptions)

func WithGetName(name string) GetOption {
	return func(opt *getOptions) {
		opt.name = name
	}
}

func WithGetAccount(account string) GetOption {
	return func(opt *getOptions) {
		opt.account = account
	}
}

func WithGetObjectKey(objectKey ObjectKeyer) GetOption {
	return func(opt *getOptions) {
		opt.name = objectKey.ObjectName()
		opt.account = objectKey.ObjectAccount()
	}
}

type getOptions struct {
	name    string
	account string
}

func (oc ObjectClient[T]) Get(
	ctx context.Context,
	opts ...GetOption,
) (*T, error) {
	opt := getOptions{}
	for _, o := range opts {
		o(&opt)
	}
	var object T
	key := ObjectKey{
		Name:    opt.name,
		Account: opt.account,
		Group:   object.ObjectGroup(),
		Kind:    object.ObjectKind(),
	}
	data, err := oc.Client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("getting object %q: %w", key, err)
	}
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
		key := KeyFromObject(t)
		opts = append(opts, WithListKey(key))
	}
	data, err := oc.Client.list(ctx, opts...)
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
	return oc.Client.Validate(
		ctx,
		WithValidateObject(object),
	)
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

	var newObj T
	data, err := json.Marshal(object)
	if err != nil {
		return newObj, fmt.Errorf("marshalling object: %w", err)
	}

	runOpts := append([]RunOption{
		WithRunObjecter(object),
		WithRunActioner(actioner),
	}, opts...)
	reply, err := oc.Client.Run(ctx, data, runOpts...)
	if err != nil {
		return newObj, fmt.Errorf("running: %w", err)
	}
	if err := json.Unmarshal(reply, &newObj); err != nil {
		return newObj, fmt.Errorf("unmarshalling reply: %w", err)
	}
	return newObj, nil
}

func SessionFromRequest(req *http.Request) string {
	if sessionCookie, err := req.Cookie(CookieSession); err == nil {
		return sessionCookie.Value
	}
	return ""
}

type ClientOption func(*clientOpts)

func WithClientInternal(b bool) ClientOption {
	return func(co *clientOpts) {
		co.internal = b
	}
}

func WithClientSession(session string) ClientOption {
	return func(co *clientOpts) {
		co.session = session
	}
}

func WithClientSessionFromRequest(req *http.Request) ClientOption {
	return func(co *clientOpts) {
		co.session = SessionFromRequest(req)
	}
}

func WithClientManager(manager string) ClientOption {
	return func(co *clientOpts) {
		co.manager = manager
	}
}

type clientOpts struct {
	internal bool
	session  string
	manager  string
}

func NewClient(conn *nats.Conn, opts ...ClientOption) Client {
	co := clientOpts{}
	for _, opt := range opts {
		opt(&co)
	}
	return Client{
		Conn:     conn,
		Internal: co.internal,
		Session:  co.session,
		Manager:  co.manager,
	}
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

	// Manager is the manager of apply operations.
	Manager string
}

func (c Client) marshalObjectWithTypeFields(obj Objecter) ([]byte, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshalling object: %w", err)
	}
	data, err = sjson.SetBytes(data, "kind", obj.ObjectKind())
	if err != nil {
		return nil, fmt.Errorf("setting kind: %w", err)
	}
	data, err = sjson.SetBytes(data, "group", obj.ObjectGroup())
	if err != nil {
		return nil, fmt.Errorf("setting group: %w", err)
	}
	data, err = sjson.SetBytes(data, "apiVersion", obj.ObjectAPIVersion())
	if err != nil {
		return nil, fmt.Errorf("setting apiVersion: %w", err)
	}
	return data, nil
}

func (c Client) checkSession() error {
	if !c.Internal && c.Session == "" {
		return ErrClientNoSession
	}
	return nil
}

func (c Client) SubjectPrefix() string {
	if c.Internal {
		return "HZ.internal."
	}
	return "HZ.api."
}

func (c Client) Schema(
	ctx context.Context,
	key ObjectKeyer,
) (Schema, error) {
	subject := c.SubjectPrefix() + fmt.Sprintf(
		SubjectStoreSchema,
		key.ObjectGroup(),
		key.ObjectKind(),
	)
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

type ValidateOption func(*validateOptions)

func WithValidateObject(obj Objecter) ValidateOption {
	return func(vo *validateOptions) {
		vo.obj = obj
	}
}

func WithValidateData(data []byte) ValidateOption {
	return func(vo *validateOptions) {
		vo.data = data
	}
}

type validateOptions struct {
	obj  Objecter
	data []byte
}

func (c Client) Validate(
	ctx context.Context,
	opts ...ValidateOption,
) error {
	vo := validateOptions{}
	for _, opt := range opts {
		opt(&vo)
	}
	var key ObjectKeyer
	if vo.obj != nil {
		var err error
		vo.data, err = c.marshalObjectWithTypeFields(vo.obj)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
		key = vo.obj
	}
	if vo.data == nil {
		return fmt.Errorf("validate: data required")
	}
	// Get key from data if it is not set.
	if key == nil {
		var metaObj MetaOnlyObject
		if err := json.Unmarshal(vo.data, &metaObj); err != nil {
			return fmt.Errorf("unmarshalling meta only object: %w", err)
		}
		key = metaObj
	}
	subject := c.SubjectPrefix() + fmt.Sprintf(
		SubjectStoreValidate,
		key.ObjectGroup(),
		key.ObjectKind(),
	)
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestWithContext(ctx, subject, vo.data)
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
	object    Objecter
	data      []byte
	objectKey *ObjectKey
	key       string
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

func WithApplyObjectKey(objectKey ObjectKey) ApplyOption {
	return func(ao *applyOptions) {
		ao.objectKey = &objectKey
	}
}

func (c Client) Apply(
	ctx context.Context,
	opts ...ApplyOption,
) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	ao := applyOptions{}
	for _, opt := range opts {
		opt(&ao)
	}

	if c.Manager == "" {
		return ErrApplyManagerRequired
	}
	if ao.object == nil && ao.data == nil {
		return ErrApplyObjectOrDataRequired
	}
	if ao.object == nil && ao.objectKey == nil {
		return ErrApplyObjectOrKeyRequired
	}

	if ao.object != nil {
		var err error
		ao.data, err = c.marshalObjectWithTypeFields(ao.object)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
		ao.key = KeyFromObject(ao.object)
	}
	if ao.objectKey != nil {
		ao.key = KeyFromObject(ao.objectKey)
	}

	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(
			SubjectStoreApply,
			ao.key,
		),
	)
	msg.Header.Set(HeaderFieldManager, c.Manager)
	msg.Header.Set(HeaderAuthorization, c.Session)
	msg.Data = ao.data
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	reply, err := c.Conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		if errors.Is(err, nats.ErrNoResponders) {
			return fmt.Errorf(
				"subject %q: %w",
				msg.Subject,
				ErrStoreNotResponding,
			)
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

type CreateOption func(*createOptions)

type createOptions struct {
	object    Objecter
	data      []byte
	objectKey *ObjectKey
	key       string
}

func WithCreateObject(object Objecter) CreateOption {
	return func(ao *createOptions) {
		ao.object = object
	}
}

func WithCreateData(data []byte) CreateOption {
	return func(ao *createOptions) {
		ao.data = data
	}
}

func WithCreateObjectKey(objectKey ObjectKey) CreateOption {
	return func(ao *createOptions) {
		ao.objectKey = &objectKey
	}
}

func (c *Client) Create(
	ctx context.Context,
	opts ...CreateOption,
) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	co := createOptions{}
	for _, opt := range opts {
		opt(&co)
	}

	if co.object == nil && co.data == nil {
		return ErrCreateObjectOrDataRequired
	}
	if co.object == nil && co.objectKey == nil {
		return ErrCreateObjectOrKeyRequired
	}

	if co.object != nil {
		var err error
		co.data, err = c.marshalObjectWithTypeFields(co.object)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
		co.key = KeyFromObject(co.object)
	}
	if co.objectKey != nil {
		co.key = KeyFromObject(co.objectKey)
	}

	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreCreate, co.key),
	)
	msg.Data = co.data
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
	key ObjectKeyer,
) ([]byte, error) {
	if err := c.checkSession(); err != nil {
		return nil, err
	}
	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreGet, KeyFromObject(key)),
	)
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
		lo.key = KeyFromObject(obj)
	}
}

type ListOption func(*listOption)

type listOption struct {
	key string
}

func (c *Client) List(
	ctx context.Context,
	opts ...ListOption,
) ([]GenericObject, error) {
	data, err := c.list(ctx, opts...)
	if err != nil {
		return nil, err
	}
	type Result struct {
		Data []GenericObject `json:"data"`
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshalling objects: %w", err)
	}
	return result.Data, nil
}

func (c *Client) list(
	ctx context.Context,
	opts ...ListOption,
) ([]byte, error) {
	if err := c.checkSession(); err != nil {
		return nil, err
	}
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

func WithRunGroup(group string) RunOption {
	return func(ro *runOption) {
		ro.group = group
	}
}

func WithRunKind(kind string) RunOption {
	return func(ro *runOption) {
		ro.kind = kind
	}
}

func WithRunAccount(account string) RunOption {
	return func(ro *runOption) {
		ro.account = account
	}
}

func WithRunName(name string) RunOption {
	return func(ro *runOption) {
		ro.name = name
	}
}

func WithRunObjecter(object ObjectKeyer) RunOption {
	return func(ro *runOption) {
		ro.group = object.ObjectGroup()
		ro.kind = object.ObjectKind()
		ro.account = object.ObjectAccount()
		ro.name = object.ObjectName()
	}
}

func WithRunAction(action string) RunOption {
	return func(ro *runOption) {
		ro.action = action
	}
}

func WithRunActioner(action Actioner) RunOption {
	return func(ro *runOption) {
		ro.action = action.Action()
	}
}

var runOptionDefault = runOption{
	timeout: time.Second * 5,
}

type runOption struct {
	timeout       time.Duration
	labelSelector LabelSelector
	group         string
	kind          string
	account       string
	name          string
	action        string
}

func (c *Client) Run(
	ctx context.Context,
	data []byte,
	opts ...RunOption,
) ([]byte, error) {
	if err := c.checkSession(); err != nil {
		return nil, err
	}
	ro := runOptionDefault
	for _, opt := range opts {
		opt(&ro)
	}
	subject := c.SubjectPrefix() + fmt.Sprintf(
		SubjectBrokeRun,
		ro.group,
		ro.kind,
		ro.account,
		ro.name,
		ro.action,
	)

	runMsg := RunMsg{
		Timeout:       ro.timeout,
		LabelSelector: ro.labelSelector,
		Data:          data,
	}
	bRunMsg, err := json.Marshal(runMsg)
	if err != nil {
		return nil, fmt.Errorf("marshalling run message: %w", err)
	}
	slog.Info("run", "subject", subject)
	ctx, cancel := context.WithTimeout(ctx, ro.timeout)
	defer cancel()
	reply, err := c.Conn.RequestWithContext(ctx, subject, bRunMsg)
	if err != nil {
		switch {
		case errors.Is(err, nats.ErrNoResponders):
			return nil, ErrRunNoResponders
		case errors.Is(err, nats.ErrTimeout):
			return nil, ErrRunTimeout
		default:
			return nil, fmt.Errorf("request: %w", err)
		}
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return nil, &Error{
			Status:  http.StatusInternalServerError,
			Message: fmt.Sprintf("invalid status header: %s", err),
		}
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusServiceUnavailable:
			return nil, ErrBrokerNoActorResponders
		case http.StatusRequestTimeout:
			return nil, ErrBrokerActorTimeout
		default:
			return nil, &Error{
				Status:  status,
				Message: string(reply.Data),
			}
		}
	}

	return reply.Data, nil
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
