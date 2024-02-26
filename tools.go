//go:build tools

package main

import (
	_ "github.com/a-h/templ/cmd/templ"

	// CI tools
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "mvdan.cc/gofumpt"
)
