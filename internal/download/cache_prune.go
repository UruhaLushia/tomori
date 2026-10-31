package download

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"
)

const cacheRetention = 30 * 24 * time.Hour

func PruneCache(ctx context.Context, dir string) error {
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
	cutoff := time.Now().Add(-cacheRetention)
	if err := pruneResourceCache(ctx, root, cutoff); err != nil {
		return err
	}
	partial, err := openCacheDirectory(root, partialDirectory)
	if err != nil || partial == nil {
		return err
	}
	defer partial.Close()
	return readCacheEntries(ctx, partial, func(entry fs.DirEntry) error {
		stem := strings.TrimSuffix(strings.TrimSuffix(entry.Name(), ".part.resume"), ".part")
		if !entry.Type().IsRegular() || len(stem) != 64 {
			return nil
		}
		if _, err := hex.DecodeString(stem); err != nil {
			return nil
		}
		names := [3]string{stem, stem + ".part", stem + ".part.resume"}
		var latest time.Time
		for _, name := range names {
			info, err := partial.Lstat(name)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || latest.IsZero() && name != entry.Name() {
				return nil
			}
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
		}
		if latest.IsZero() || !latest.Before(cutoff) {
			return nil
		}
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := partial.Remove(name); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	})
}

func pruneResourceCache(ctx context.Context, parent *os.Root, cutoff time.Time) error {
	root, err := openCacheDirectory(parent, ResourceCache)
	if err != nil || root == nil {
		return err
	}
	defer root.Close()
	return readCacheEntries(ctx, root, func(entry fs.DirEntry) error {
		if !entry.IsDir() || len(entry.Name()) != 40 {
			return nil
		}
		if _, err := hex.DecodeString(entry.Name()); err != nil {
			return nil
		}
		child, err := openCacheDirectory(root, entry.Name())
		if err != nil || child == nil {
			return err
		}
		defer child.Close()
		info, err := child.Stat(".")
		if err != nil {
			return err
		}
		latest := info.ModTime()
		if err := readCacheEntries(ctx, child, func(file fs.DirEntry) error {
			if info, err := child.Lstat(file.Name()); err == nil && info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		}); err != nil {
			return err
		}
		if latest.Before(cutoff) {
			if err := ctx.Err(); err != nil {
				return err
			}
			return root.RemoveAll(entry.Name())
		}
		return nil
	})
}

func openCacheDirectory(root *os.Root, name string) (*os.Root, error) {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}
	return root.OpenRoot(name)
}

func readCacheEntries(ctx context.Context, root *os.Root, visit func(fs.DirEntry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		entries, err := dir.ReadDir(64)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(entry); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return ctx.Err()
		}
		if err != nil {
			return err
		}
	}
}
