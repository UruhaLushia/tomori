//go:build !linux && !windows && !darwin

package network

import (
	"context"

	"golang.org/x/net/http/httpproxy"
)

func nativeProxy(context.Context) httpproxy.Config { return httpproxy.Config{} }
