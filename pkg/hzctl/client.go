package hzctl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/verifa/horizon/pkg/hz"
)

// Client is an HTTP client for interacting with a Horizon server.
//
// It is intended for CLIs and other tooling for end users.
// If you are building a service or controller, use the [hz.Client] instead
// which uses a NATS directly, and not the HTTP API.
type Client struct {
	Server  string
	Session string
	Manager string
}

type ListOption func(*getOptions)

func WithListKey(key hz.ObjectKey) ListOption {
	return func(opt *getOptions) {
		opt.key = key
	}
}

func WithListResponseWriter(w io.Writer) ListOption {
	return func(opt *getOptions) {
		opt.respWriter = w
	}
}

func WithListResponseGenericObject(
	resp *hz.GenericObjectList,
) ListOption {
	return func(opt *getOptions) {
		opt.respGenericObjects = resp
	}
}

type getOptions struct {
	key hz.ObjectKey

	respWriter         io.Writer
	respGenericObjects *hz.GenericObjectList
}

func (c *Client) List(ctx context.Context, opts ...ListOption) error {
	opt := getOptions{}
	for _, o := range opts {
		o(&opt)
	}
	reqURL, err := url.JoinPath(c.Server, "v1", "objects")
	if err != nil {
		return fmt.Errorf("creating request url: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set(hz.HeaderAuthorization, c.Session)

	q := req.URL.Query()
	if opt.key.Group != "" {
		q.Add("group", opt.key.Group)
	}
	if opt.key.Kind != "" {
		q.Add("kind", opt.key.Kind)
	}
	if opt.key.Account != "" {
		q.Add("account", opt.key.Account)
	}
	if opt.key.Name != "" {
		q.Add("name", opt.key.Name)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if err := hz.ErrorFromHTTP(resp); err != nil {
		return err
	}
	if opt.respWriter != nil {
		if _, err := io.Copy(opt.respWriter, resp.Body); err != nil {
			return fmt.Errorf("writing response: %w", err)
		}
	}
	if opt.respGenericObjects != nil {
		if err := json.NewDecoder(resp.Body).Decode(opt.respGenericObjects); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}
	return nil
}

type ApplyOption func(*applyOptions)

func WithApplyObject(object hz.Objecter) ApplyOption {
	return func(opt *applyOptions) {
		opt.object = object
	}
}

func WithApplyData(data []byte) ApplyOption {
	return func(opt *applyOptions) {
		opt.data = data
	}
}

type applyOptions struct {
	object hz.Objecter
	data   []byte
}

func (c *Client) Apply(
	ctx context.Context,
	opts ...ApplyOption,
) (hz.ApplyOpResult, error) {
	ao := applyOptions{}
	for _, o := range opts {
		o(&ao)
	}

	reqURL, err := url.JoinPath(c.Server, "v1", "objects")
	if err != nil {
		return hz.ApplyOpResultError, fmt.Errorf(
			"creating request url: %w",
			err,
		)
	}

	req, err := http.NewRequest(
		http.MethodPatch,
		reqURL,
		bytes.NewReader(ao.data),
	)
	if err != nil {
		return hz.ApplyOpResultError, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add(hz.HeaderAuthorization, c.Session)
	req.Header.Add(hz.HeaderApplyFieldManager, c.Manager)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return hz.ApplyOpResultError, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		return hz.ApplyOpResultCreated, nil
	case http.StatusOK:
		return hz.ApplyOpResultUpdated, nil
	case http.StatusNotModified:
		return hz.ApplyOpResultNoop, nil
	case http.StatusConflict:
		return hz.ApplyOpResultConflict, hz.ErrorFromHTTP(resp)
	default:
		return hz.ApplyOpResultError, hz.ErrorFromHTTP(resp)
	}
}

type DeleteOption func(*deleteOptions)

func WithDeleteKey(key hz.ObjectKeyer) DeleteOption {
	return func(opt *deleteOptions) {
		opt.key = key
	}
}

func WithDeleteData(data []byte) DeleteOption {
	return func(opt *deleteOptions) {
		opt.data = data
	}
}

type deleteOptions struct {
	key  hz.ObjectKeyer
	data []byte
}

func (c *Client) Delete(
	ctx context.Context,
	opts ...DeleteOption,
) error {
	opt := deleteOptions{}
	for _, o := range opts {
		o(&opt)
	}
	var key hz.ObjectKeyer
	if opt.key != nil {
		key = opt.key
	}
	if opt.data != nil {
		var obj hz.MetaOnlyObject
		if err := json.Unmarshal(opt.data, &obj); err != nil {
			return fmt.Errorf("unmarshaling object: %w", err)
		}
		key = obj
	}
	if key == nil {
		return fmt.Errorf("delete: key required")
	}

	if _, err := hz.KeyFromObjectStrict(key); err != nil {
		return fmt.Errorf("delete: invalid key: %w", err)
	}
	reqURL, err := url.JoinPath(
		c.Server,
		"v1",
		"objects",
		key.ObjectGroup(),
		key.ObjectVersion(),
		key.ObjectKind(),
		key.ObjectAccount(),
		key.ObjectName(),
	)
	if err != nil {
		return fmt.Errorf("creating request url: %w", err)
	}
	req, err := http.NewRequest(
		http.MethodDelete,
		reqURL,
		bytes.NewReader(opt.data),
	)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add(hz.HeaderAuthorization, c.Session)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if err := hz.ErrorFromHTTP(resp); err != nil {
		return err
	}
	return nil
}
