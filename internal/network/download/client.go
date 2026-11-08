package download

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
)

type Client struct {
	HTTP             *httpclient.Client
	Rewrite          func(string) string
	UserAgent        string
	Workers          int
	Connections      int
	Retries          int
	Segmented        bool
	SegmentThreshold int64
	CacheDir         string
	Budget           *Budget
	// PrepareRequest applies credentials to the actual destination, including
	// redirects and requests issued by the segmented downloader.
	PrepareRequest func(*http.Request)
	httpOnce       sync.Once
	budgetOnce     sync.Once
	shared         *Budget
	limitedHTTP    *http.Client
	received       atomic.Int64
}

type Task struct {
	URL     string
	Mirrors []string
	Path    string
	SHA1    string
	SHA512  string
	Size    int64
	MaxSize int64
}

func NewClient(client *httpclient.Client) *Client {
	return &Client{
		Workers:          16,
		Connections:      8,
		Retries:          3,
		Segmented:        true,
		SegmentThreshold: 8 << 20,
		HTTP:             client,
	}
}

func (f *Client) httpTimeout() time.Duration {
	if f.HTTP != nil && f.HTTP.Timeout > 0 {
		return f.HTTP.Timeout
	}
	return 5 * time.Minute
}

func (f *Client) agent() string {
	if f.UserAgent == "" {
		return "Tomori/0.1.0"
	}
	return f.UserAgent
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

func TotalSize(tasks []Task) int64 {
	var total int64
	for _, t := range tasks {
		total += max(0, t.Size)
	}
	return total
}

func (f *Client) rewrite(address string) string {
	if f.Rewrite != nil {
		return f.Rewrite(address)
	}
	return address
}
