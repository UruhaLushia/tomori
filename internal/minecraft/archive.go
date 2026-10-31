package minecraft

import (
	"context"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
)

func extractZip(ctx context.Context, path, dir string, exclude []string) error {
	r, err := fsutil.OpenZip(ctx, path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	var bytes uint64
	for _, file := range r.File {
		if !file.FileInfo().IsDir() && !excluded(file.Name, exclude) {
			bytes = storage.Add(bytes, file.UncompressedSize64)
		}
	}
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: dir, Bytes: bytes}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	w, err := fsutil.NewArchiveWriter(dir, 10000, 256<<20, 1<<30)
	if err != nil {
		return err
	}
	defer func() { _ = w.Close() }()
	for _, f := range r.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if excluded(f.Name, exclude) {
			continue
		}
		if err := w.WriteZip(ctx, f, f.Name); err != nil {
			return err
		}
	}
	return nil
}

func excluded(name string, patterns []string) bool {
	for _, p := range patterns {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}
