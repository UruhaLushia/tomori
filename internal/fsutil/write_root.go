package fsutil

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
)

func WriteRootAtomic(ctx context.Context, root *os.Root, name string, raw []byte) error {
	return WriteRootAtomicStream(ctx, root, name, func(w io.Writer) error {
		_, err := w.Write(raw)
		return err
	})
}

func WriteRootAtomicStream(ctx context.Context, root *os.Root, name string, write func(io.Writer) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := copyPath(name); err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	temp := filepath.Join(filepath.Dir(name), ".tomori-write-"+rand.Text())
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temp) }()
	err = write(f)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Rename(temp, name)
}
