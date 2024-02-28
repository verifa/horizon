package hz

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	HeaderStatus              = "Hz-Status"
	HeaderAuthorization       = "Hz-Authorization"
	HeaderApplyFieldManager   = "Hz-Apply-Field-Manager"
	HeaderApplyForceConflicts = "Hz-Apply-Force-Conflicts"
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
	ErrClientObjectOrDataRequired = errors.New(
		"client: object or data required",
	)
	ErrClientObjectOrKeyRequired = errors.New("client: object or key required")

	ErrClientNoSession = errors.New("client: no session")

	ErrStoreNotResponding = errors.New("store: not responding")

	ErrRunNoResponders         = errors.New("run: no brokers responding")
	ErrRunTimeout              = errors.New("run: broker timeout")
	ErrBrokerNoActorResponders = errors.New("broker: no actor responders")
	ErrBrokerActorTimeout      = errors.New("broker: actor timeout")
)

const (
	SubjectAPIAllowAll = "HZ.api.>"

	// format: HZ.api.broker.<group>.<kind>.<account>.<name>.<action>
	SubjectAPIBroker                  = "HZ.api.broker.*.*.*.*.*"
	SubjectInternalBroker             = "HZ.internal.broker.*.*.*.*.*"
	SubjectInternalBrokerIndexGroup   = 3
	SubjectInternalBrokerIndexKind    = 4
	SubjectInternalBrokerIndexAccount = 5
	SubjectInternalBrokerIndexName    = 6
	SubjectInternalBrokerIndexAction  = 7
	SubjectInternalBrokerLength       = 8
	SubjectBrokeRun                   = "broker.%s.%s"

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
	SubjectStoreDelete   = "store.delete.%s"
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

func (oc ObjectClient[T]) Get(
	ctx context.Context,
	opts ...GetOption,
) (T, error) {
	opt := getOptions{}
	for _, o := range opts {
		o(&opt)
	}
	var object T
	key := ObjectKey{
		Name:    opt.key.ObjectName(),
		Account: opt.key.ObjectAccount(),
		Group:   object.ObjectGroup(),
		Kind:    object.ObjectKind(),
	}
	opts = append(opts, WithGetKey(key))
	data, err := oc.Client.Get(ctx, opts...)
	if err != nil {
		return object, fmt.Errorf("getting object %q: %w", key, err)
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return object, fmt.Errorf("unmarshalling object: %w", err)
	}
	return object, nil
}

func (oc ObjectClient[T]) List(
	ctx context.Context,
	opts ...ListOption,
) ([]*T, error) {
	opt := listOption{}
	for _, o := range opts {
		o(&opt)
	}

	var t T
	key := ObjectKey{
		Group: t.ObjectGroup(),
		Kind:  t.ObjectKind(),
	}
	if opt.key != nil {
		key.Name = opt.key.ObjectName()
		key.Account = opt.key.ObjectAccount()
	}
	resp := bytes.Buffer{}
	opts = append(opts, WithListKey(key), WithListResponseWriter(&resp))
	if err := oc.Client.List(ctx, opts...); err != nil {
		return nil, fmt.Errorf("listing objects: %w", err)
	}

	var result TypedObjectList[T]
	if err := json.NewDecoder(&resp).Decode(&result); err != nil {
		return nil, fmt.Errorf("unmarshalling objects: %w", err)
	}

	return result.Items, nil
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
	return req.Header.Get(HeaderAuthorization)
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
	data, err = sjson.SetBytes(
		data,
		"apiVersion",
		fmt.Sprintf("%s/%s", obj.ObjectGroup(), obj.ObjectVersion()),
	)
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
	object Objecter
	data   []byte
	key    ObjectKeyer
	force  bool
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

func WithApplyKey(key ObjectKeyer) ApplyOption {
	return func(ao *applyOptions) {
		ao.key = key
	}
}

func WithApplyForce(force bool) ApplyOption {
	return func(ao *applyOptions) {
		ao.force = force
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
	var (
		key  string
		data []byte
	)
	if ao.object != nil {
		var err error
		data, err = c.marshalObjectWithTypeFields(ao.object)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
		key, err = KeyFromObjectConcrete(ao.object)
		if err != nil {
			return fmt.Errorf("invalid object: %w", err)
		}
	}
	if ao.key != nil {
		var err error
		key, err = KeyFromObjectConcrete(ao.key)
		if err != nil {
			return fmt.Errorf("invalid object: %w", err)
		}
	}
	if ao.data != nil {
		var obj EmptyObjectWithMeta
		if err := json.Unmarshal(ao.data, &obj); err != nil {
			return fmt.Errorf("unmarshalling data: %w", err)
		}
		var err error
		key, err = KeyFromObjectConcrete(obj)
		if err != nil {
			return fmt.Errorf("invalid data: %w", err)
		}
		data = ao.data
	}
	if key == "" {
		return fmt.Errorf("apply: %w", ErrClientObjectOrKeyRequired)
	}
	if data == nil {
		return fmt.Errorf("apply: %w", ErrClientObjectOrDataRequired)
	}
	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(
			SubjectStoreApply,
			key,
		),
	)
	msg.Header.Set(HeaderApplyFieldManager, c.Manager)
	msg.Header.Set(HeaderApplyForceConflicts, strconv.FormatBool(ao.force))
	msg.Header.Set(HeaderAuthorization, c.Session)
	msg.Data = data
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
	return ErrorFromNATS(reply)
}

type CreateOption func(*createOptions)

type createOptions struct {
	object Objecter
	data   []byte
	key    *ObjectKey
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

func WithCreateKey(objectKey ObjectKey) CreateOption {
	return func(ao *createOptions) {
		ao.key = &objectKey
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

	var (
		key  string
		data []byte
	)
	if co.object != nil {
		var err error
		key, err = KeyFromObjectConcrete(co.object)
		if err != nil {
			return fmt.Errorf("invalid object: %w", err)
		}

		data, err = c.marshalObjectWithTypeFields(co.object)
		if err != nil {
			return fmt.Errorf("marshalling object: %w", err)
		}
	}
	if co.key != nil {
		var err error
		key, err = KeyFromObjectConcrete(co.key)
		if err != nil {
			return fmt.Errorf("invalid key: %w", err)
		}
	}
	if co.data != nil {
		var obj EmptyObjectWithMeta
		if err := json.Unmarshal(co.data, &obj); err != nil {
			return fmt.Errorf("unmarshalling data: %w", err)
		}
		var err error
		key, err = KeyFromObjectConcrete(obj)
		if err != nil {
			return fmt.Errorf("invalid data: %w", err)
		}
		data = co.data
	}
	if key == "" {
		return fmt.Errorf("create: %w", ErrClientObjectOrKeyRequired)
	}
	if data == nil {
		return fmt.Errorf("create: %w", ErrClientObjectOrDataRequired)
	}

	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreCreate, key),
	)
	msg.Data = data
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
	return ErrorFromNATS(reply)
}

type GetOption func(*getOptions)

func WithGetKey(key ObjectKeyer) GetOption {
	return func(opt *getOptions) {
		opt.key = key
	}
}

type getOptions struct {
	key ObjectKeyer
}

func (c *Client) Get(
	ctx context.Context,
	opts ...GetOption,
) ([]byte, error) {
	if err := c.checkSession(); err != nil {
		return nil, err
	}
	opt := getOptions{}
	for _, o := range opts {
		o(&opt)
	}
	var key string
	if opt.key != nil {
		var err error
		key, err = KeyFromObjectConcrete(opt.key)
		if err != nil {
			return nil, fmt.Errorf("invalid key: %w", err)
		}
	}

	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreGet, key),
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
	return reply.Data, ErrorFromNATS(reply)
}

type DeleteOption func(*deleteOptions)

func WithDeleteObject(object Objecter) DeleteOption {
	return func(do *deleteOptions) {
		do.object = object
	}
}

func WithDeleteData(data []byte) DeleteOption {
	return func(do *deleteOptions) {
		do.data = data
	}
}

func WithDeleteKey(key ObjectKeyer) DeleteOption {
	return func(do *deleteOptions) {
		do.key = key
	}
}

type deleteOptions struct {
	key    ObjectKeyer
	object Objecter
	data   []byte
}

func (c *Client) Delete(
	ctx context.Context,
	opts ...DeleteOption,
) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	do := deleteOptions{}
	for _, opt := range opts {
		opt(&do)
	}
	var key string
	if do.object != nil {
		var err error
		key, err = KeyFromObjectConcrete(do.object)
		if err != nil {
			return fmt.Errorf("invalid object: %w", err)
		}
	}
	if do.key != nil {
		var err error
		key, err = KeyFromObjectConcrete(do.key)
		if err != nil {
			return fmt.Errorf("invalid key: %w", err)
		}
	}
	if do.data != nil {
		var obj EmptyObjectWithMeta
		if err := json.Unmarshal(do.data, &obj); err != nil {
			return fmt.Errorf("unmarshalling data: %w", err)
		}
		var err error
		key, err = KeyFromObjectConcrete(obj)
		if err != nil {
			return fmt.Errorf("invalid data: %w", err)
		}
	}
	if key == "" {
		return fmt.Errorf("delete: key required")
	}
	msg := nats.NewMsg(
		c.SubjectPrefix() + fmt.Sprintf(SubjectStoreDelete, key),
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
			return ErrStoreNotResponding
		}
		return fmt.Errorf("making request to store: %w", err)
	}
	return ErrorFromNATS(reply)
}

func WithListKey(obj ObjectKeyer) ListOption {
	return func(lo *listOption) {
		lo.key = obj
	}
}

func WithListResponseWriter(w io.Writer) ListOption {
	return func(lo *listOption) {
		lo.responseWriter = w
	}
}

func WithListResponseGenericObjects(resp *GenericObjectList) ListOption {
	return func(lo *listOption) {
		lo.responseGenericObjectList = resp
	}
}

type ListOption func(*listOption)

type listOption struct {
	key ObjectKeyer

	responseWriter            io.Writer
	responseGenericObjectList *GenericObjectList
}

func (c *Client) List(
	ctx context.Context,
	opts ...ListOption,
) error {
	if err := c.checkSession(); err != nil {
		return err
	}
	lo := listOption{}
	for _, opt := range opts {
		opt(&lo)
	}
	var key string
	if lo.key != nil {
		key = KeyFromObject(lo.key)
	}
	if key == "" {
		return fmt.Errorf("list: key required")
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
			return ErrStoreNotResponding
		}
		return fmt.Errorf("making request to store: %w", err)
	}
	status, err := strconv.Atoi(reply.Header.Get(HeaderStatus))
	if err != nil {
		return fmt.Errorf("invalid status header: %w", err)
	}
	if status != http.StatusOK {
		return &Error{
			Status:  status,
			Message: string(reply.Data),
		}
	}
	if lo.responseWriter != nil {
		_, err := lo.responseWriter.Write(reply.Data)
		if err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}
	if lo.responseGenericObjectList != nil {
		if err := json.Unmarshal(reply.Data, lo.responseGenericObjectList); err != nil {
			return fmt.Errorf("unmarshalling objects: %w", err)
		}
	}

	return nil
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

func WithRunObjecter(key ObjectKeyer) RunOption {
	return func(ro *runOption) {
		ro.key = key
	}
}

func WithRunActioner(action Actioner) RunOption {
	return func(ro *runOption) {
		ro.actioner = action
	}
}

var runOptionDefault = runOption{
	timeout: time.Second * 5,
}

// TODO: use key
type runOption struct {
	timeout       time.Duration
	labelSelector LabelSelector
	key           ObjectKeyer
	actioner      Actioner
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
	var (
		key    string
		action string
	)
	if ro.key != nil {
		var err error
		key, err = KeyFromObjectConcrete(ro.key)
		if err != nil {
			return nil, fmt.Errorf("invalid key: %w", err)
		}
	}
	if ro.actioner != nil {
		action = ro.actioner.Action()
	}
	if key == "" {
		return nil, fmt.Errorf("run: key required")
	}
	if action == "" {
		return nil, fmt.Errorf("run: action required")
	}
	subject := c.SubjectPrefix() + fmt.Sprintf(
		SubjectBrokeRun,
		key,
		action,
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
