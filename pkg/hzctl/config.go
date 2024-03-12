package hzctl

import (
	"errors"
	"fmt"
	"slices"
)

// Config represents the configuration file for the Horizon CLI.
// It contains a list of context and the current context.
type Config struct {
	CurrentContext string    `json:"currentContext"`
	Contexts       []Context `json:"contexts"`
}

// Context represents a Horizon context.
// It contains the name, URL and session for a Horizon server.
type Context struct {
	Name string `json:"name,omitempty"`

	URL     string  `json:"url,omitempty"`
	Session *string `json:"session,omitempty"`
}

type ValidateOption func(*validateOptions)

// WithValidateSession returns a ValidateOption that sets the session validation
// to the given bool.
func WithValidateSession(b bool) ValidateOption {
	return func(o *validateOptions) {
		o.hasSession = b
	}
}

type validateOptions struct {
	hasSession bool
}

// Validate validates the context based on the given validation options.
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

// WithContextTryName returns a ContextOption that sets the context name to the
// given string, if it is not nil nor empty.
// If the given string is nil or empty, it does nothing.
func WithContextTryName(name *string) ContextOption {
	return func(o *contextOptions) {
		if name != nil && *name != "" {
			o.byName = *name
			o.useCurrent = false
		}
	}
}

// WithContextCurrent returns a ContextOption that sets the context to the
// current context based on the given bool.
func WithContextCurrent(b bool) ContextOption {
	return func(o *contextOptions) {
		o.useCurrent = b
	}
}

// WithContextByName returns a ContextOption that sets the context to the one
// with the given name.
func WithContextByName(name string) ContextOption {
	return func(o *contextOptions) {
		o.useCurrent = false
		o.byName = name
	}
}

// WithContextValidate returns a ContextOption that sets the context validation
// options to the given ValidateOptions.
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

// Context returns a context based on the given context options, or an error if
// no such context was found.
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

// Add adds a context to the config.
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
