package proxy

import (
	"context"
	"sync"

	"golang.org/x/net/http/httpproxy"
)

var systemProxyState struct {
	sync.RWMutex
	once   sync.Once
	config *httpproxy.Config
}

func WatchSystemProxy(ctx context.Context, changed func()) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		err := watchNativeProxy(ctx, changed)
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func systemProxyConfig(ctx context.Context) *httpproxy.Config {
	systemProxyState.once.Do(func() {
		updateSystemProxy(systemProxy(ctx))
		go func() {
			_ = WatchSystemProxy(context.Background(), func() {
				updateSystemProxy(systemProxy(context.Background()))
			})
		}()
	})
	systemProxyState.RLock()
	defer systemProxyState.RUnlock()
	if systemProxyState.config == nil {
		return &httpproxy.Config{}
	}
	config := *systemProxyState.config
	return &config
}

func updateSystemProxy(config *httpproxy.Config) {
	systemProxyState.Lock()
	systemProxyState.config = config
	systemProxyState.Unlock()
}
