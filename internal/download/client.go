package download

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/network"
	"github.com/UruhaLushia/tomori/locales"
)

type Client struct {
	HTTP             *http.Client
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

func NewClient(proxy network.Proxy) *Client {
	transport := network.Transport(proxy)
	return &Client{
		Workers:          16,
		Connections:      8,
		Retries:          3,
		Segmented:        true,
		SegmentThreshold: 8 << 20,
		HTTP: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Minute,
		},
	}
}

func (f *Client) httpTimeout() time.Duration {
	if f.HTTP != nil && f.HTTP.Timeout > 0 {
		return f.HTTP.Timeout
	}
	return 5 * time.Minute
}

func (f *Client) Get(ctx context.Context, url string) ([]byte, error) {
	return f.GetLimited(ctx, url, 64<<20)
}

func (f *Client) GetLimited(ctx context.Context, url string, limit int64) ([]byte, error) {
	if limit <= 0 || limit > 64<<20 {
		return nil, errors.New(locales.InvalidMetadataSizeLimit())
	}
	rewritten := f.rewrite(url)
	body, err := f.requestLimit(ctx, http.MethodGet, rewritten, nil, nil, limit)
	if err != nil && rewritten != url && ctx.Err() == nil {
		return f.requestLimit(ctx, http.MethodGet, url, nil, nil, limit)
	}
	return body, err
}

func (f *Client) PostJSONLimited(ctx context.Context, url string, body, result any, limit int64) error {
	if limit <= 0 || limit > 64<<20 {
		return errors.New(locales.InvalidMetadataSizeLimit())
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	raw, err = f.requestLimit(ctx, http.MethodPost, url, raw, nil, limit)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func (f *Client) JSONHeaders(ctx context.Context, method, url string, body any, headers map[string]string, result any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	raw, err := f.requestLimit(ctx, method, url, payload, headers, 64<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, result)
}

func (f *Client) requestLimit(ctx context.Context, method, url string, payload []byte, headers map[string]string, limit int64) ([]byte, error) {
	var last error
	for attempt := 0; attempt <= f.Retries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", f.agent())
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := f.httpClient().Do(req)
		if err != nil {
			last = err
		} else {
			if resp.StatusCode != http.StatusOK {
				if resp.ContentLength >= 0 && resp.ContentLength <= 4<<10 {
					_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
				}
				_ = resp.Body.Close()
				last = fmt.Errorf("%s: %s", url, resp.Status)
				if resp.StatusCode < 500 && resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
					return nil, last
				}
			} else {
				var body []byte
				if resp.ContentLength > limit {
					_ = resp.Body.Close()
					return nil, fmt.Errorf(locales.MetadataTooLargeAlt(), limit, url)
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
					return nil, fmt.Errorf(locales.MetadataTooLargeAlt(), limit, url)
				} else {
					return body, nil
				}
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == f.Retries {
			break
		}
		if err := sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond); err != nil {
			return nil, err
		}
	}
	return nil, last
}

func (f *Client) GetJSON(ctx context.Context, url string, v any) error {
	return f.GetJSONLimited(ctx, url, v, 64<<20)
}

func (f *Client) GetJSONLimited(ctx context.Context, url string, v any, limit int64) error {
	body, err := f.GetLimited(ctx, url, limit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("%s: %w", url, err)
	}
	return nil
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

func (f *Client) rewrite(url string) string {
	if f.Rewrite != nil {
		return f.Rewrite(url)
	}
	return url
}
