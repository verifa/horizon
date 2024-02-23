package hzctl

import "slices"

type Config struct {
	CurrentContext string    `json:"currentContext"`
	Contexts       []Context `json:"contexts"`
}

type Context struct {
	Name string `json:"name,omitempty"`

	URL         string  `json:"url,omitempty"`
	Account     *string `json:"account,omitempty"`
	Credentials *string `json:"credentials,omitempty"`
	Session     *string `json:"session,omitempty"`
}

func (c *Config) Current() (Context, bool) {
	for _, ctx := range c.Contexts {
		if ctx.Name == c.CurrentContext {
			return ctx, true
		}
	}
	return Context{}, false
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
