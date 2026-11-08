package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/locales"
)

type Budget struct {
	bandwidth                 bandwidth
	files, network, throttled permits
	mu                        sync.Mutex
	locks                     map[string]*pathLock
}

type permits struct {
	mu            sync.Mutex
	limit, active int
	wake          chan struct{}
}

func NewBudget(files, connections int) *Budget {
	b := &Budget{locks: make(map[string]*pathLock)}
	b.bandwidth.gate, b.bandwidth.wake = make(chan struct{}, 1), make(chan struct{}, 1)
	b.bandwidth.changed = make(chan struct{})
	b.files.wake, b.network.wake = make(chan struct{}, 1), make(chan struct{}, 1)
	b.throttled.wake = make(chan struct{}, 1)
	b.Configure(files, connections)
	return b
}

func (b *Budget) Configure(files, connections int) {
	b.files.configure(files)
	b.network.configure(connections)
	b.throttled.configure(max(1, min(connections, 32)-1))
}

func (p *permits) configure(limit int) {
	p.mu.Lock()
	p.limit = max(1, min(limit, 32))
	p.mu.Unlock()
	p.notify()
}

func (p *permits) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *permits) take(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.mu.Lock()
		if p.active < p.limit {
			p.active++
			more := p.active < p.limit
			p.mu.Unlock()
			if more {
				p.notify()
			}
			return nil
		}
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.wake:
		}
	}
}

func (p *permits) give() {
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	p.notify()
}

type pathLock struct {
	gate     chan struct{}
	refs     int
	identity string
}

func (f *Client) budget() *Budget {
	f.budgetOnce.Do(func() {
		f.shared = f.Budget
		if f.shared == nil {
			f.shared = NewBudget(f.Workers, max(f.Workers, f.Connections))
		}
	})
	return f.shared
}

func (f *Client) Lock(ctx context.Context, scope, path string) (func(), error) {
	return f.budget().Lock(ctx, scope, path)
}

// FilePermit covers local file work; release it before starting a download.
func (f *Client) FilePermit(ctx context.Context) (func(), error) {
	return f.budget().FilePermit(ctx)
}

func (b *Budget) FilePermit(ctx context.Context) (func(), error) {
	if err := b.files.take(ctx); err != nil {
		return nil, err
	}
	return sync.OnceFunc(b.files.give), nil
}

func (b *Budget) NetworkPermit(ctx context.Context) (func(), error) {
	if err := b.network.take(ctx); err != nil {
		return nil, err
	}
	return sync.OnceFunc(b.network.give), nil
}

func (b *Budget) Lock(ctx context.Context, scope, path string) (func(), error) {
	return b.lock(ctx, scope, path, "")
}

func (b *Budget) lock(ctx context.Context, scope, path, identity string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(path)
		if err == nil {
			path = resolved
			break
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil, err
		}
		suffix = append(suffix, filepath.Base(path))
		path = parent
	}
	for _, s := range slices.Backward(suffix) {
		path = filepath.Join(path, s)
	}
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	key := scope + "\x00" + path
	b.mu.Lock()
	lock := b.locks[key]
	if lock == nil {
		lock = &pathLock{gate: make(chan struct{}, 1), identity: identity}
		b.locks[key] = lock
	} else if lock.identity != identity {
		b.mu.Unlock()
		return nil, fmt.Errorf(locales.DownloadTargetConflict(), path)
	}
	lock.refs++
	b.mu.Unlock()
	drop := func() {
		b.mu.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(b.locks, key)
		}
		b.mu.Unlock()
	}
	select {
	case lock.gate <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-lock.gate
			drop()
			return nil, err
		}
		return sync.OnceFunc(func() { <-lock.gate; drop() }), nil
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
}
