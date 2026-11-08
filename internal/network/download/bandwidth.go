package download

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type bandwidth struct {
	rate    atomic.Int64
	gate    chan struct{}
	wake    chan struct{}
	timer   *time.Timer
	mu      sync.Mutex
	tokens  float64
	at      time.Time
	changed chan struct{}
}

func (b *Budget) SetSpeedLimit(bytesPerSecond int64) {
	l := &b.bandwidth
	l.mu.Lock()
	defer l.mu.Unlock()
	bytesPerSecond = max(0, min(bytesPerSecond, 1<<30))
	if l.rate.Load() == bytesPerSecond {
		return
	}
	l.rate.Store(bytesPerSecond)
	l.tokens, l.at = 0, time.Now()
	close(l.changed)
	l.changed = make(chan struct{})
}

func (b *Budget) LimitReader(ctx context.Context, in io.Reader) io.Reader {
	return &bandwidthReader{ctx: ctx, in: in, limit: &b.bandwidth}
}

func (l *bandwidth) take(ctx context.Context, size int) (int, error) {
	if l.rate.Load() == 0 {
		return size, ctx.Err()
	}
	select {
	case l.gate <- struct{}{}:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	defer func() { <-l.gate }()
	defer func() {
		if l.timer != nil {
			l.timer.Stop()
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		l.mu.Lock()
		rate := l.rate.Load()
		if rate == 0 {
			l.mu.Unlock()
			return size, nil
		}
		now := time.Now()
		l.tokens = min(float64(max(1, min(rate/10, 1<<20))), l.tokens+now.Sub(l.at).Seconds()*float64(rate))
		l.at = now
		n := min(size, 64<<10, int(max(1, rate/20)))
		if l.tokens >= float64(n) {
			l.tokens -= float64(n)
			l.mu.Unlock()
			return n, nil
		}
		delay := max(time.Microsecond, time.Duration((float64(n)-l.tokens)/float64(rate)*float64(time.Second)))
		changed := l.changed
		l.mu.Unlock()
		if l.timer == nil {
			l.timer = time.NewTimer(delay)
		} else {
			l.timer.Reset(delay)
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-changed:
		case <-l.wake:
		case <-l.timer.C:
		}
		l.timer.Stop()
	}
}

func (l *bandwidth) refund(n int) {
	if n <= 0 {
		return
	}
	l.mu.Lock()
	rate := l.rate.Load()
	if rate > 0 {
		l.tokens = min(float64(max(1, min(rate/10, 1<<20))), l.tokens+float64(n))
	}
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

type bandwidthReader struct {
	ctx   context.Context
	in    io.Reader
	limit *bandwidth
}

func (r *bandwidthReader) Read(p []byte) (int, error) {
	if len(p) == 0 || r.limit.rate.Load() == 0 {
		return r.in.Read(p)
	}
	allowed, err := r.limit.take(r.ctx, len(p))
	if err != nil {
		return 0, err
	}
	n, err := r.in.Read(p[:allowed])
	r.limit.refund(allowed - n)
	return n, err
}

type fileRequestKey struct{}
