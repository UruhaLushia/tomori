//go:build !linux && !windows && !darwin

package proxy

import (
	"context"

	"golang.org/x/net/http/httpproxy"
)

func nativeProxy(context.Context) httpproxy.Config { return httpproxy.Config{} }
