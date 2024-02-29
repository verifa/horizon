package hz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// TODO: this should probably go in its own package?
type HTTPClient struct {
	Server  string
	Session string
	Manager string
}

type HTTPListOption func(*httpGetOptions)

func WithHTTPListKey(key ObjectKey) HTTPListOption {
	return func(opt *httpGetOptions) {
		opt.key = key
	}
}

func WithHTTPListResponseWriter(w io.Writer) HTTPListOption {
	return func(opt *httpGetOptions) {
		opt.respWriter = w
	}
}

func WithHTTPListResponseGenericObject(resp *GenericObjectList) HTTPListOption {
	return func(opt *httpGetOptions) {
		opt.respGenericObjects = resp
	}
}

type httpGetOptions struct {
	key ObjectKey

	respWriter         io.Writer
	respGenericObjects *GenericObjectList
}

func (c *HTTPClient) List(ctx context.Context, opts ...HTTPListOption) error {
	opt := httpGetOptions{}
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
	req.Header.Set(HeaderAuthorization, c.Session)

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

	if err := ErrorFromHTTP(resp); err != nil {
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

type HTTPApplyOption func(*httpApplyOptions)

func WithHTTPApplyObject(object Objecter) HTTPApplyOption {
	return func(opt *httpApplyOptions) {
		opt.object = object
	}
}

func WithHTTPApplyData(data []byte) HTTPApplyOption {
	return func(opt *httpApplyOptions) {
		opt.data = data
	}
}

type httpApplyOptions struct {
	object Objecter
	data   []byte
}

func (c *HTTPClient) Apply(ctx context.Context, opts ...HTTPApplyOption) error {
	ao := httpApplyOptions{}
	for _, o := range opts {
		o(&ao)
	}

	reqURL, err := url.JoinPath(c.Server, "v1", "objects")
	if err != nil {
		return fmt.Errorf("creating request url: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPatch,
		reqURL,
		bytes.NewReader(ao.data),
	)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add(HeaderAuthorization, c.Session)
	req.Header.Add(HeaderApplyFieldManager, c.Manager)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if err := ErrorFromHTTP(resp); err != nil {
		return err
	}
	return nil
}

type HTTPDeleteOption func(*httpDeleteOptions)

func WithHTTPDeleteKey(key ObjectKeyer) HTTPDeleteOption {
	return func(opt *httpDeleteOptions) {
		opt.key = key
	}
}

func WithHTTPDeleteData(data []byte) HTTPDeleteOption {
	return func(opt *httpDeleteOptions) {
		opt.data = data
	}
}

type httpDeleteOptions struct {
	key  ObjectKeyer
	data []byte
}

func (c *HTTPClient) Delete(
	ctx context.Context,
	opts ...HTTPDeleteOption,
) error {
	opt := httpDeleteOptions{}
	for _, o := range opts {
		o(&opt)
	}
	var key ObjectKeyer
	if opt.key != nil {
		key = opt.key
	}
	if opt.data != nil {
		var obj MetaOnlyObject
		if err := json.Unmarshal(opt.data, &obj); err != nil {
			return fmt.Errorf("unmarshaling object: %w", err)
		}
		key = obj
	}
	if key == nil {
		return fmt.Errorf("delete: key required")
	}

	if _, err := KeyFromObjectStrict(key); err != nil {
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
	req.Header.Add(HeaderAuthorization, c.Session)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if err := ErrorFromHTTP(resp); err != nil {
		return err
	}
	return nil
}
