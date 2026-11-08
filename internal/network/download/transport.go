package download

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
)

func (f *Client) httpClient() *http.Client {
	f.httpOnce.Do(func() {
		base := f.HTTP
		if base == nil {
			base = &httpclient.Client{Client: http.DefaultClient}
		}
		client := *base.Client
		transport := base.Transport
		if transport == nil {
			transport = http.DefaultTransport
		}
		limit := max(1, min(max(f.Workers, f.Connections), 32))
		if t, ok := transport.(*http.Transport); ok {
			t = t.Clone()
			t.MaxConnsPerHost, t.MaxIdleConnsPerHost = limit, limit
			t.MaxIdleConns = max(64, limit*4)
			t.ResponseHeaderTimeout = 20 * time.Second
			t.TLSHandshakeTimeout = 10 * time.Second
			transport = t
		}
		prepare := f.PrepareRequest
		if prepare == nil && f.HTTP != nil {
			prepare = f.HTTP.PrepareRequest
		}
		client.Transport = &limitedTransport{base: transport, slots: make(chan struct{}, limit), budget: f.budget(), timeout: client.Timeout, received: &f.received, prepare: prepare}
		// Waiting for a download slot is not part of the network timeout.
		client.Timeout = 0
		f.limitedHTTP = &client
	})
	return f.limitedHTTP
}

func (f *Client) segmentedClients(count int) []*http.Client {
	client := f.httpClient()
	limited := client.Transport.(*limitedTransport)
	transport, ok := limited.base.(*http.Transport)
	if !ok {
		return []*http.Client{client}
	}
	clients := make([]*http.Client, 0, count)
	for range count {
		// Separate pools keep HTTP/2 from multiplexing every segment over one
		// TCP connection. All workers still share permits and bandwidth limits.
		pool := transport.Clone()
		pool.MaxConnsPerHost, pool.MaxIdleConnsPerHost, pool.MaxIdleConns = 1, 1, 1
		workerTransport := *limited
		workerTransport.base = pool
		worker := *client
		worker.Transport = &workerTransport
		clients = append(clients, &worker)
	}
	return clients
}

type limitedTransport struct {
	base     http.RoundTripper
	slots    chan struct{}
	budget   *Budget
	timeout  time.Duration
	received *atomic.Int64
	prepare  func(*http.Request)
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	file, _ := req.Context().Value(fileRequestKey{}).(bool)
	throttled := file && t.budget.bandwidth.rate.Load() > 0
	if throttled {
		if err := t.budget.throttled.take(req.Context()); err != nil {
			return nil, err
		}
	}
	select {
	case t.slots <- struct{}{}:
	case <-req.Context().Done():
		if throttled {
			t.budget.throttled.give()
		}
		return nil, req.Context().Err()
	}
	if err := t.budget.network.take(req.Context()); err != nil {
		if throttled {
			t.budget.throttled.give()
		}
		<-t.slots
		return nil, err
	}
	var ctx context.Context
	var cancel context.CancelFunc
	var timer *time.Timer
	if file {
		var cause context.CancelCauseFunc
		ctx, cause = context.WithCancelCause(req.Context())
		cancel = func() { cause(context.Canceled) }
		if t.timeout > 0 {
			timer = time.AfterFunc(t.timeout, func() { cause(context.DeadlineExceeded) })
		}
	} else if t.timeout > 0 {
		ctx, cancel = context.WithTimeout(req.Context(), t.timeout)
	} else {
		ctx, cancel = context.WithCancel(req.Context())
	}
	release := sync.OnceFunc(func() {
		if timer != nil {
			timer.Stop()
		}
		cancel()
		t.budget.network.give()
		if throttled {
			t.budget.throttled.give()
		}
		<-t.slots
	})
	req = req.Clone(ctx)
	if t.prepare != nil {
		t.prepare(req)
	}
	resp, err := t.base.RoundTrip(req)
	if timer != nil {
		timer.Stop()
	}
	if err != nil {
		if context.Cause(ctx) == context.DeadlineExceeded {
			err = context.DeadlineExceeded
		}
		release()
		return nil, err
	}
	// Mojang's Azure CDN can return unquoted ETags. They are invalid If-Range
	// validators and make the server return the whole file instead of a range.
	// Discard them so range readers can use Last-Modified instead.
	if etag := resp.Header.Get("ETag"); etag != "" && !validEntityTag(etag) {
		resp.Header.Del("ETag")
	}
	var reader io.Reader = resp.Body
	if file {
		if timer != nil {
			reader = &timedReader{in: reader, timer: timer, timeout: t.timeout}
		}
		reader = t.budget.LimitReader(ctx, reader)
	}
	resp.Body = &limitedBody{ReadCloser: resp.Body, reader: reader, ctx: ctx, release: release, received: t.received}
	return resp, nil
}

func validEntityTag(value string) bool {
	tag, _ := strings.CutPrefix(value, "W/")
	if len(tag) < 2 || tag[0] != '"' || tag[len(tag)-1] != '"' {
		return false
	}
	for i := 1; i < len(tag)-1; i++ {
		if b := tag[i]; b < 0x21 || b == '"' || b == 0x7f {
			return false
		}
	}
	return true
}

func (t *limitedTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

type limitedBody struct {
	io.ReadCloser
	reader   io.Reader
	ctx      context.Context
	release  func()
	received *atomic.Int64
}

func (b *limitedBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	if err != nil && context.Cause(b.ctx) == context.DeadlineExceeded {
		err = context.DeadlineExceeded
	}
	b.received.Add(int64(n))
	if err != nil {
		b.release()
	}
	return n, err
}

type timedReader struct {
	in      io.Reader
	timer   *time.Timer
	timeout time.Duration
}

func (r *timedReader) Read(p []byte) (int, error) {
	r.timer.Reset(r.timeout)
	n, err := r.in.Read(p)
	r.timer.Stop()
	return n, err
}

func (b *limitedBody) Close() error {
	err := b.ReadCloser.Close()
	b.release()
	return err
}
