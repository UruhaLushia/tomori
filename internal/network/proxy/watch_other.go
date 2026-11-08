//go:build !linux && !windows && !darwin

package proxy

import (
	"context"
)

func watchNativeProxy(ctx context.Context, _ func()) error {
	<-ctx.Done()
	return ctx.Err()
}
