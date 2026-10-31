package fsutil

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

// Publish supports caches on another filesystem without exposing a partially
// copied destination. The verified source is retained when publishing fails.
func Publish(ctx context.Context, src, dst string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.CreateTemp(filepath.Dir(dst), ".tomori-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(out.Name()) }()
	_, err = io.Copy(out, contextReader{ctx, in})
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(out.Name(), dst); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func ContextReader(ctx context.Context, reader io.Reader) io.Reader {
	return contextReader{ctx, reader}
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
