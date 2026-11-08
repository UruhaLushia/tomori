package download

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

var (
	ErrRangeUnsupported = errors.New("remote server does not support ranges")
	ErrPreviewLimit     = errors.New("remote preview exceeds metadata budget")
)

type remoteRegion struct {
	offset int64
	data   []byte
}

type RangeReader struct {
	mu                  sync.Mutex
	client              *Client
	ctx                 context.Context
	url, etag, modified string
	size, received      int64
	requests            int
	regions             []remoteRegion
}

func (f *Client) OpenRange(ctx context.Context, url string, size int64) (*RangeReader, error) {
	offset, length := int64(0), int64(1)
	if size > 0 {
		length = min(size, 65557)
		offset = size - length
	}
	r := &RangeReader{client: f, ctx: ctx, url: f.rewrite(url)}
	err := r.fetch(offset, length)
	if err != nil && r.url != url && ctx.Err() == nil {
		r = &RangeReader{client: f, ctx: ctx, url: url}
		err = r.fetch(offset, length)
	}
	return r, err
}

func (r *RangeReader) Size() int64 { return r.size }

func (r *RangeReader) ReadAt(p []byte, offset int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if offset < 0 {
		return 0, errors.New("invalid remote offset")
	}
	written := 0
	for len(p) > 0 && offset < r.size {
		copied := 0
		for _, region := range r.regions {
			if offset >= region.offset && offset-region.offset < int64(len(region.data)) {
				copied = copy(p, region.data[offset-region.offset:])
				break
			}
		}
		if copied == 0 {
			length := min(r.size-offset, max(int64(len(p)), 32<<10))
			if err := r.fetch(offset, length); err != nil {
				return written, err
			}
			continue
		}
		written, offset, p = written+copied, offset+int64(copied), p[copied:]
	}
	if len(p) > 0 {
		return written, io.EOF
	}
	return written, nil
}

func (r *RangeReader) fetch(offset, length int64) error {
	if r.requests >= 64 || length <= 0 || length > (8<<20)-r.received {
		return ErrPreviewLimit
	}
	r.requests++
	req, err := http.NewRequestWithContext(r.ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", r.client.agent())
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+length-1))
	if r.etag != "" {
		req.Header.Set("If-Range", r.etag)
	} else if r.modified != "" {
		req.Header.Set("If-Range", r.modified)
	}
	resp, err := r.client.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		if resp.StatusCode == http.StatusOK {
			return ErrRangeUnsupported
		}
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return ErrRangeUnsupported
	}
	rangeValue, valid := strings.CutPrefix(resp.Header.Get("Content-Range"), "bytes ")
	if !valid {
		return ErrRangeUnsupported
	}
	span, total, ok := strings.Cut(rangeValue, "/")
	start, end, spanOK := strings.Cut(span, "-")
	from, e1 := strconv.ParseInt(start, 10, 64)
	to, e2 := strconv.ParseInt(end, 10, 64)
	size, e3 := strconv.ParseInt(total, 10, 64)
	if !ok || !spanOK || e1 != nil || e2 != nil || e3 != nil || from < 0 || to < from || size <= to || size > 64<<30 {
		return ErrRangeUnsupported
	}
	if from != offset || to != offset+length-1 || r.size != 0 && size != r.size {
		return ErrRangeUnsupported
	}
	count := to - from + 1
	if count > length || resp.ContentLength >= 0 && resp.ContentLength != count {
		return ErrRangeUnsupported
	}
	etag, modified := resp.Header.Get("ETag"), resp.Header.Get("Last-Modified")
	if r.size != 0 && (r.etag != "" && etag != r.etag || r.modified != "" && modified != r.modified) {
		return ErrRangeUnsupported
	}
	data, err := fsutil.ReadExact(r.ctx, resp.Body, count)
	if err != nil {
		return err
	}
	if r.size == 0 {
		r.size = size
		if etag != "" && !strings.HasPrefix(etag, "W/") {
			r.etag = etag
		} else {
			r.modified = modified
		}
	}
	r.received += count
	r.regions = append(r.regions, remoteRegion{offset: from, data: data})
	return nil
}
