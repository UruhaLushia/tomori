package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// File maps an immutable source identity to an opaque cache file. The cache
// never needs a human-readable name: the source is the lookup key and the
// file can always be regenerated when it is removed.
func File(dir, source string) string {
	sum := sha256.Sum256([]byte(source))
	return filepath.Join(dir, hex.EncodeToString(sum[:]))
}

func Clear(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func Size(ctx context.Context, dir string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer root.Close()
	var size uint64
	err = fs.WalkDir(root.FS(), ".", func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 0 {
			size += uint64(info.Size())
		}
		return nil
	})
	return size, err
}

const retention = 30 * 24 * time.Hour

func Prune(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	cutoff := time.Now().Add(-retention)
	return fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().Before(cutoff) {
			return root.Remove(path)
		}
		return nil
	})
}
