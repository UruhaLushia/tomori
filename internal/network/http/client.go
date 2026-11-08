package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"time"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/network/proxy"
	"github.com/UruhaLushia/tomori/locales"
)

// Permit lets the caller share its network budget without coupling this
// package to the file downloader.
type Permit interface {
	NetworkPermit(context.Context) (func(), error)
}

// Client is the ordinary HTTP client used by API/catalog callers. File
// transfers belong to network/download and should use that package instead.
type Client struct {
	*nethttp.Client
	Rewrite        func(string) string
	UserAgent      string
	Retries        int
	MetadataCache  *cache.Store
	Budget         Permit
	PrepareRequest func(*nethttp.Request)
	queue          resourceQueue
}

func New(p proxy.Proxy, timeout time.Duration) *Client {
	return &Client{
		Client:  &nethttp.Client{Transport: proxy.Transport(p), Timeout: timeout},
		Retries: 3,
	}
}

func (c *Client) Clone() *Client {
	clone := *c.Client
	return &Client{
		Client:         &clone,
		Rewrite:        c.Rewrite,
		UserAgent:      c.UserAgent,
		Retries:        c.Retries,
		MetadataCache:  c.MetadataCache,
		Budget:         c.Budget,
		PrepareRequest: c.PrepareRequest,
	}
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.CloseIdleConnections()
	c.queue.close(func() {
		if c.MetadataCache != nil {
			_ = c.MetadataCache.Close()
		}
	})
}

func (c *Client) Do(req *nethttp.Request) (*nethttp.Response, error) {
	if c.PrepareRequest != nil {
		c.PrepareRequest(req)
	}
	release, err := c.network(req.Context())
	if err != nil {
		return nil, err
	}
	defer release()
	if c.PrepareRequest == nil {
		return c.Client.Do(req)
	}
	client := *c.Client
	redirect := client.CheckRedirect
	client.CheckRedirect = func(next *nethttp.Request, via []*nethttp.Request) error {
		c.PrepareRequest(next)
		if redirect != nil {
			return redirect(next, via)
		}
		return nil
	}
	return client.Do(req)
}

func (c *Client) network(ctx context.Context) (func(), error) {
	if c.Budget == nil {
		return func() {}, ctx.Err()
	}
	return c.Budget.NetworkPermit(ctx)
}

func (c *Client) Get(ctx context.Context, address string) ([]byte, error) {
	return c.GetLimited(ctx, address, 64<<20)
}

func (c *Client) GetLimited(ctx context.Context, address string, limit int64) ([]byte, error) {
	if err := validLimit(limit); err != nil {
		return nil, err
	}
	rewritten := c.rewrite(address)
	if body, ok := c.cachedMetadata(ctx, rewritten, limit); ok {
		return body, nil
	}
	return c.queue.get(c, ctx, rewritten, address, limit, false)
}

const (
	metadataFresh = 10 * time.Minute
	metadataStale = 7 * 24 * time.Hour
)

func (c *Client) cachedMetadata(ctx context.Context, address string, limit int64) ([]byte, bool) {
	if c.MetadataCache == nil {
		return nil, false
	}
	body, updated, ok, err := c.MetadataCache.Get(ctx, address)
	if err != nil || !ok || len(body) == 0 || int64(len(body)) > limit {
		return nil, false
	}
	age := max(time.Since(updated), 0)
	if age <= metadataFresh {
		return body, true
	}
	if age <= metadataStale {
		c.refreshMetadata(address, limit)
		return body, true
	}
	return nil, false
}

func (c *Client) storeMetadata(ctx context.Context, address string, body []byte) {
	if c.MetadataCache != nil {
		_ = c.MetadataCache.Put(ctx, address, body)
	}
}

func (c *Client) refreshMetadata(address string, limit int64) {
	if c.MetadataCache == nil {
		return
	}
	release := c.MetadataCache.Hold()
	if release == nil {
		return
	}
	if _, loaded := c.queue.refresh.LoadOrStore(address, struct{}{}); loaded {
		release()
		return
	}
	c.queue.mu.Lock()
	if c.queue.closed {
		c.queue.mu.Unlock()
		c.queue.refresh.Delete(address)
		release()
		return
	}
	c.queue.active.Add(1)
	c.queue.mu.Unlock()
	go func() {
		defer release()
		defer c.queue.refresh.Delete(address)
		defer c.queue.active.Done()
		ctx, cancel := context.WithTimeout(context.Background(), min(c.timeout(), 30*time.Second))
		defer cancel()
		body, err := c.requestLimit(ctx, nethttp.MethodGet, address, nil, nil, limit)
		if err == nil {
			c.storeMetadata(ctx, address, body)
		}
	}()
}

func (c *Client) PostJSONLimited(ctx context.Context, address string, body, result any, limit int64) error {
	if limit <= 0 || limit > 64<<20 {
		return errors.New(locales.InvalidMetadataSizeLimit())
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	raw, err := c.requestLimit(ctx, nethttp.MethodPost, address, payload, nil, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func (c *Client) JSONHeaders(ctx context.Context, method, address string, body any, headers map[string]string, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	raw, err := c.requestLimit(ctx, method, address, payload, headers, 64<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func (c *Client) GetJSON(ctx context.Context, address string, result any) error {
	return c.GetJSONLimited(ctx, address, result, 64<<20)
}

func (c *Client) GetJSONLimited(ctx context.Context, address string, result any, limit int64) error {
	body, err := c.GetLimited(ctx, address, limit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("%s: %w", address, err)
	}
	return nil
}

func (c *Client) requestLimit(ctx context.Context, method, address string, payload []byte, headers map[string]string, limit int64) ([]byte, error) {
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		req, err := nethttp.NewRequestWithContext(ctx, method, address, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.agent())
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.Do(req)
		if err != nil {
			last = err
		} else if resp.StatusCode != nethttp.StatusOK {
			if resp.ContentLength >= 0 && resp.ContentLength <= 4<<10 {
				_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
			}
			_ = resp.Body.Close()
			last = fmt.Errorf("%s: %s", address, resp.Status)
			if resp.StatusCode < 500 && resp.StatusCode != nethttp.StatusRequestTimeout && resp.StatusCode != nethttp.StatusTooManyRequests {
				return nil, last
			}
		} else {
			var body []byte
			if resp.ContentLength > limit {
				_ = resp.Body.Close()
				return nil, fmt.Errorf(locales.MetadataTooLargeAlt(), limit, address)
			}
			if resp.ContentLength >= 0 {
				body, err = fsutil.ReadExact(ctx, resp.Body, resp.ContentLength)
			} else {
				body, err = io.ReadAll(io.LimitReader(resp.Body, limit+1))
			}
			_ = resp.Body.Close()
			if err != nil {
				last = err
			} else if int64(len(body)) > limit {
				return nil, fmt.Errorf(locales.MetadataTooLargeAlt(), limit, address)
			} else {
				return body, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == c.Retries {
			break
		}
		if err := sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond); err != nil {
			return nil, err
		}
	}
	return nil, last
}

func (c *Client) agent() string {
	if c.UserAgent == "" {
		return "Tomori/0.1.0"
	}
	return c.UserAgent
}

func (c *Client) timeout() time.Duration {
	if c.Client != nil && c.Timeout > 0 {
		return c.Timeout
	}
	return 5 * time.Minute
}

func (c *Client) rewrite(address string) string {
	if c.Rewrite != nil {
		return c.Rewrite(address)
	}
	return address
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
