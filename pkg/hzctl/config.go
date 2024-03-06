package hzctl

import (
	"errors"
	"fmt"
	"slices"
)

type Config struct {
	CurrentContext string    `json:"currentContext"`
	Contexts       []Context `json:"contexts"`
}

type Context struct {
	Name string `json:"name,omitempty"`

	URL     string  `json:"url,omitempty"`
	Account *string `json:"account,omitempty"`
	Session *string `json:"session,omitempty"`
}

type ValidateOption func(*validateOptions)

func WithValidateSession(b bool) ValidateOption {
	return func(o *validateOptions) {
		o.hasSession = b
		o.hasCredentials = b
	}
}

type validateOptions struct {
	hasSession     bool
	hasCredentials bool
}

func (c *Context) Validate(opts ...ValidateOption) error {
	vOpts := &validateOptions{}
	for _, opt := range opts {
		opt(vOpts)
	}
	var errs error
	if c.Name == "" {
		errs = errors.Join(errs, fmt.Errorf("context name is required"))
	}
	if c.URL == "" {
		errs = errors.Join(errs, fmt.Errorf("context url is required"))
	}
	if vOpts.hasSession && c.Session == nil {
		errs = errors.Join(errs, fmt.Errorf("context session is required"))
	}
	return errs
}

type ContextOption func(*contextOptions)

func WithContextTryName(name *string) ContextOption {
	return func(o *contextOptions) {
		if name != nil && *name != "" {
			o.byName = *name
			o.useCurrent = false
		}
	}
}

func WithContextCurrent(b bool) ContextOption {
	return func(o *contextOptions) {
		o.useCurrent = b
	}
}

func WithContextByName(name string) ContextOption {
	return func(o *contextOptions) {
		o.useCurrent = false
		o.byName = name
	}
}

func WithContextValidate(opts ...ValidateOption) ContextOption {
	return func(o *contextOptions) {
		o.validateOpts = opts
	}
}

type contextOptions struct {
	useCurrent bool
	byName     string

	validateOpts []ValidateOption
}

func (c *Config) Context(opts ...ContextOption) (Context, error) {
	cOpts := &contextOptions{}
	for _, opt := range opts {
		opt(cOpts)
	}
	for _, ctx := range c.Contexts {
		if ctx.Name == c.CurrentContext {
			if err := ctx.Validate(cOpts.validateOpts...); err != nil {
				return Context{}, fmt.Errorf(
					"validating current context: %w",
					err,
				)
			}
			return ctx, nil
		}
	}
	return Context{}, errors.New("current context not found")
}

func (c *Config) Add(ctx Context) {
	index := slices.IndexFunc(c.Contexts, func(hCtx Context) bool {
		return hCtx.Name == ctx.Name
	})
	if index == -1 {
		c.Contexts = append(c.Contexts, ctx)
		if len(c.Contexts) == 1 {
			c.CurrentContext = ctx.Name
		}
		return
	}
	c.Contexts[index] = ctx
}
