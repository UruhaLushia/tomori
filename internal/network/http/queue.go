package httpclient

import (
	"context"
	"errors"
	nethttp "net/http"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type resourceQueue struct {
	refresh sync.Map
	pending sync.Map
	mu      sync.Mutex
	closed  bool
	active  sync.WaitGroup
}

type pendingResource struct {
	done chan struct{}
	body []byte
	err  error
}

// GetResource shares an in-flight fetch for a cacheable binary resource. The
// fetch continues briefly after the caller is canceled so a page switch does
// not discard a resource that another page is about to use.
func (c *Client) GetResource(ctx context.Context, address string, limit int64) ([]byte, error) {
	if err := validLimit(limit); err != nil {
		return nil, err
	}
	rewritten := c.rewrite(address)
	if body, ok := c.cachedMetadata(ctx, rewritten, limit); ok {
		return body, nil
	}
	return c.queue.get(c, ctx, rewritten, address, limit, true)
}

// RefreshResource bypasses the cached body while retaining request merging.
// Callers use it when a cached response fails format validation.
func (c *Client) RefreshResource(ctx context.Context, address string, limit int64) ([]byte, error) {
	if err := validLimit(limit); err != nil {
		return nil, err
	}
	rewritten := c.rewrite(address)
	return c.queue.get(c, ctx, rewritten, address, limit, true)
}

func (q *resourceQueue) get(c *Client, ctx context.Context, rewritten, original string, limit int64, detached bool) ([]byte, error) {
	pending := &pendingResource{done: make(chan struct{})}
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return nil, context.Canceled
	}
	actual, loaded := q.pending.LoadOrStore(rewritten, pending)
	if !loaded {
		q.active.Add(1)
	}
	q.mu.Unlock()
	if loaded {
		pending = actual.(*pendingResource)
	} else if detached {
		go c.fetchResource(rewritten, original, limit, pending)
	} else {
		c.fetchResourceWithContext(ctx, rewritten, original, limit, pending)
	}
	select {
	case <-pending.done:
		return pending.body, pending.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Client) fetchResource(rewritten, original string, limit int64, pending *pendingResource) {
	ctx, cancel := context.WithTimeout(context.Background(), min(c.timeout(), 2*time.Minute))
	defer cancel()
	c.fetchResourceWithContext(ctx, rewritten, original, limit, pending)
}

func (c *Client) fetchResourceWithContext(ctx context.Context, rewritten, original string, limit int64, pending *pendingResource) {
	defer c.queue.active.Done()
	body, err := c.requestLimit(ctx, nethttp.MethodGet, rewritten, nil, nil, limit)
	if err != nil && rewritten != original && ctx.Err() == nil {
		body, err = c.requestLimit(ctx, nethttp.MethodGet, original, nil, nil, limit)
	}
	if err == nil {
		c.storeMetadata(ctx, rewritten, body)
	}
	pending.body, pending.err = body, err
	close(pending.done)
	c.queue.pending.Delete(rewritten)
}

func (q *resourceQueue) close(after func()) {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		return
	}
	q.closed = true
	q.mu.Unlock()
	go func() {
		q.active.Wait()
		after()
	}()
}

func validLimit(limit int64) error {
	if limit <= 0 || limit > 64<<20 {
		return errors.New(locales.InvalidMetadataSizeLimit())
	}
	return nil
}
