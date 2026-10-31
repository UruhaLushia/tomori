package download

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/piko"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (f *Client) fetchSegmented(ctx context.Context, url string, t Task, progress func(int64)) error {
	// The final destination is replaced only after Mojang's hash and size match.
	output := f.segmentedPath(t)
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	if taskComplete(ctx, t, output) {
		return fsutil.Publish(ctx, output, t.Path)
	}
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: filepath.Dir(output), Bytes: uint64(t.Size)}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	opts := piko.DefaultOptions()
	opts.Output, opts.UserAgent = output, f.agent()
	opts.Connections, opts.Retries = max(1, min(f.Connections, 32)), 0
	opts.HTTPClients = f.segmentedClients(opts.Connections)
	defer func() {
		for _, client := range opts.HTTPClients {
			client.CloseIdleConnections()
		}
	}()
	opts.Timeout = f.httpTimeout()
	opts.Resume, opts.Force = true, true
	// An explicit range prevents piko from selecting its single-stream path.
	opts.Length = t.Size
	opts.Progress = func(p piko.Progress) {
		if progress != nil {
			progress(p.Bytes)
		}
	}
	if _, err := piko.Download(ctx, url, opts); err != nil {
		return err
	}
	if !taskComplete(ctx, t, output) {
		if err := ctx.Err(); err != nil {
			return err
		}
		_ = os.Remove(output)
		return fmt.Errorf(locales.SegmentedDownloadVerificationFailed(), filepath.Base(t.Path))
	}
	return fsutil.Publish(ctx, output, t.Path)
}
