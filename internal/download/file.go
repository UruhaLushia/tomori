package download

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (f *Client) Fetch(ctx context.Context, t Task) error {
	return f.fetchFrom(ctx, t, nil, nil, false)
}

type LocalSources func(Task) []string

func (f *Client) FetchWithReuse(ctx context.Context, t Task, sources LocalSources) error {
	return f.fetchFrom(ctx, t, nil, sources, false)
}

func (f *Client) ReuseLocal(ctx context.Context, t Task, sources LocalSources) error {
	return f.fetchFrom(ctx, t, nil, sources, true)
}

// Close releases idle connections after a complete install or manifest operation.
func (f *Client) Close() { f.httpClient().CloseIdleConnections() }

type byteProgress struct {
	bytes  int64
	last   int64
	at     time.Time
	notify func(int64)
}

func (p *byteProgress) Write(b []byte) (int, error) {
	p.bytes += int64(len(b))
	if p.notify != nil && (p.bytes-p.last >= 256<<10 || time.Since(p.at) >= 100*time.Millisecond) {
		p.last, p.at = p.bytes, time.Now()
		p.notify(p.bytes)
	}
	return len(b), nil
}

var copyBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

func (f *Client) fetchFrom(ctx context.Context, t Task, progress func(int64), sources LocalSources, localOnly bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.MaxSize > 0 && t.Size > t.MaxSize {
		return fileTooLarge(t)
	}
	identity := strings.ToLower(t.SHA1)
	if t.SHA512 != "" {
		identity += ":" + strings.ToLower(t.SHA512)
	}
	if identity == "" {
		identity = t.URL
	}
	budget := f.budget()
	release, err := budget.lock(ctx, "download", t.Path, identity)
	if err != nil {
		return err
	}
	defer release()
	if !localOnly && f.Segmented && t.Size >= max(1<<20, f.SegmentThreshold) && f.Connections > 1 {
		release, err := f.Lock(ctx, "segments", f.segmentedPath(t))
		if err != nil {
			return err
		}
		defer release()
	}
	if err := budget.files.take(ctx); err != nil {
		return err
	}
	defer budget.files.give()
	if taskComplete(ctx, t, t.Path) {
		return nil
	}
	if t.Size > 0 {
		report, err := storage.Assess(ctx, []storage.Requirement{{Directory: filepath.Dir(t.Path), Bytes: uint64(t.Size)}})
		if err != nil {
			return err
		}
		if err := report.Check(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(t.Path), 0o755); err != nil {
		return err
	}
	if sources != nil && (t.SHA1 != "" || t.SHA512 != "") {
		for _, source := range sources(t) {
			if err := ctx.Err(); err != nil {
				return err
			}
			in, err := fsutil.OpenRegular(source)
			if err != nil {
				continue
			}
			info, err := in.Stat()
			if err != nil || (t.Size > 0 && info.Size() != t.Size && t.SHA1 == "" && t.SHA512 == "") || (t.MaxSize > 0 && info.Size() > t.MaxSize) {
				_ = in.Close()
				continue
			}
			err = copyVerified(ctx, t, in, nil)
			_ = in.Close()
			if err == nil {
				return nil
			}
			var invalid *verificationError
			if !errors.As(err, &invalid) {
				return err
			}
		}
	}
	if localOnly {
		return errors.New(locales.LocalFileIsMissingOrFailedVerification())
	}
	if progress != nil {
		progress(0)
	}
	var last error
	for _, source := range append([]string{t.URL}, t.Mirrors...) {
		url := f.rewrite(source)
		last = f.fetchURL(ctx, url, t, progress)
		if storage.IsFull(last) {
			return last
		}
		if last != nil && url != source && ctx.Err() == nil {
			last = f.fetchURL(ctx, source, t, progress)
		}
		if storage.IsFull(last) {
			return last
		}
		if last == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return last
}

func (f *Client) fetchURL(ctx context.Context, url string, t Task, progress func(int64)) error {
	ctx = context.WithValue(ctx, fileRequestKey{}, true)
	var last error
	segmented := f.Segmented && t.Size >= max(1<<20, f.SegmentThreshold) && f.Connections > 1
	for attempt := 0; attempt <= f.Retries; attempt++ {
		var err error
		if segmented {
			err = f.fetchSegmented(ctx, url, t, progress)
			if err != nil && ctx.Err() == nil && !storage.IsFull(err) {
				// A source can pass the range probe but ignore ranges on later requests.
				// Keep subsequent retries on the ordinary HTTP path for this URL.
				segmented = false
				err = f.fetchOnce(ctx, url, t, progress)
			}
		} else {
			err = f.fetchOnce(ctx, url, t, progress)
		}
		if err == nil {
			f.removeSegmentedFiles(t)
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return err
		}
		if storage.IsFull(err) {
			return err
		}
		last = err
		var response *responseError
		var sizeLimit *sizeLimitError
		if errors.As(err, &sizeLimit) {
			break
		}
		if errors.As(err, &response) && response.code >= 400 && response.code < 500 && response.code != http.StatusRequestTimeout && response.code != http.StatusTooManyRequests {
			break
		}
		if attempt == f.Retries {
			break
		}
		if err := sleep(ctx, time.Duration(attempt+1)*500*time.Millisecond); err != nil {
			return err
		}
	}
	return fmt.Errorf("%s: %w", t.URL, last)
}

func (f *Client) fetchOnce(ctx context.Context, url string, t Task, progress func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", f.agent())
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := f.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &responseError{code: resp.StatusCode, status: resp.Status}
	}
	if t.MaxSize > 0 && resp.ContentLength > t.MaxSize {
		return fileTooLarge(t)
	}
	if t.Size <= 0 && resp.ContentLength > 0 {
		report, err := storage.Assess(ctx, []storage.Requirement{{Directory: filepath.Dir(t.Path), Bytes: uint64(resp.ContentLength)}})
		if err != nil {
			return err
		}
		if err := report.Check(); err != nil {
			return err
		}
	}
	return copyVerified(ctx, t, resp.Body, progress)
}

func copyVerified(ctx context.Context, t Task, source io.Reader, progress func(int64)) error {
	tmp, err := os.CreateTemp(filepath.Dir(t.Path), ".download-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	var hash1, hash512 hash.Hash
	var writer io.Writer = tmp
	if t.SHA1 != "" {
		hash1 = sha1.New()
		writer = io.MultiWriter(writer, hash1)
	}
	if t.SHA512 != "" {
		hash512 = sha512.New()
		writer = io.MultiWriter(writer, hash512)
	}
	if progress != nil {
		writer = io.MultiWriter(writer, &byteProgress{notify: progress})
	}
	buffer := copyBuffers.Get().(*[]byte)
	reader := fsutil.ContextReader(ctx, source)
	limit := t.Size
	if t.SHA1 != "" || t.SHA512 != "" {
		limit = 0
	}
	if t.MaxSize > 0 && (limit <= 0 || t.MaxSize < limit) {
		limit = t.MaxSize
	}
	if limit > 0 {
		reader = io.LimitReader(reader, limit+1)
	}
	n, err := io.CopyBuffer(writer, reader, *buffer)
	copyBuffers.Put(buffer)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if t.MaxSize > 0 && n > t.MaxSize {
		return fileTooLarge(t)
	}
	sha1OK := t.SHA1 == "" || strings.EqualFold(hex.EncodeToString(hash1.Sum(nil)), t.SHA1)
	sha512OK := t.SHA512 == "" || strings.EqualFold(hex.EncodeToString(hash512.Sum(nil)), t.SHA512)
	if !sha1OK {
		return &verificationError{fmt.Sprintf(locales.VerificationFailed(), filepath.Base(t.Path))}
	}
	if !sha512OK {
		return &verificationError{fmt.Sprintf(locales.Sha512Mismatch(), filepath.Base(t.Path))}
	}
	// A trusted checksum remains authoritative when upstream size metadata is stale.
	if t.Size > 0 && n != t.Size && t.SHA1 == "" && t.SHA512 == "" {
		return &verificationError{fmt.Sprintf(locales.SizeVerificationFailed(), filepath.Base(t.Path), n, t.Size)}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(name, t.Path)
}

func taskComplete(ctx context.Context, t Task, path string) bool {
	if t.MaxSize > 0 {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > t.MaxSize {
			return false
		}
	}
	if fsutil.CompleteHashesContext(ctx, path, t.SHA1, t.SHA512, t.Size) {
		return true
	}
	// The checksum is the content identity. Allow an existing file with a
	// trusted checksum to be reused when upstream metadata has a stale size.
	return t.Size > 0 && (t.SHA1 != "" || t.SHA512 != "") && fsutil.CompleteHashesContext(ctx, path, t.SHA1, t.SHA512, 0)
}

type sizeLimitError struct{ message string }

func (e *sizeLimitError) Error() string { return e.message }

func fileTooLarge(t Task) error {
	return &sizeLimitError{fmt.Sprintf(locales.DownloadExceedsSizeLimit(), filepath.Base(t.Path))}
}

type verificationError struct{ message string }

func (e *verificationError) Error() string { return e.message }

type responseError struct {
	code   int
	status string
}

func (e *responseError) Error() string { return e.status }
