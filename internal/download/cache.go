package download

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const ManifestCache = ".tomori-version-manifest.json"
const ResourceCache = ".tomori-resources"
const LoaderCache = ".tomori-loaders"
const AvatarCache = ".tomori-avatars"
const AppearanceCache = ".tomori-appearance"
const AuthlibCache = ".tomori-authlib"
const NamesCache = ".tomori-resource-names.csv"
const partialDirectory = ".tomori-downloads"

var cacheEntries = [...]struct {
	name      string
	directory bool
}{
	{AuthlibCache, true},
	{NamesCache, false},
	{AvatarCache, true},
	{AppearanceCache, true},
	{LoaderCache, true},
	{ResourceCache, true},
	{partialDirectory, true},
	{ManifestCache, false},
}

func (f *Client) segmentedPath(t Task) string {
	if f.CacheDir == "" {
		return t.Path + ".tomori-download"
	}
	identity := t.URL + "\x00" + t.Path + "\x00" + t.SHA1 + fmt.Sprintf("\x00%d", t.Size)
	if t.SHA512 != "" {
		identity += "\x00" + t.SHA512
	}
	key := sha256.Sum256([]byte(identity))
	return filepath.Join(f.CacheDir, partialDirectory, fmt.Sprintf("%x", key))
}

// ClearCache removes only launcher-owned entries, even when the chosen cache
// directory contains files belonging to other applications.
func ClearCache(dir string) error {
	for _, entry := range cacheEntries {
		remove := os.Remove
		if entry.directory {
			remove = os.RemoveAll
		}
		if err := remove(filepath.Join(dir, entry.name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// CacheSize counts launcher-owned files without following symbolic links.
func CacheSize(ctx context.Context, dir string) (uint64, error) {
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
	for _, entry := range cacheEntries {
		err := fs.WalkDir(root.FS(), entry.name, func(_ string, entry fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if os.IsNotExist(err) {
				return nil
			}
			if err != nil {
				return err
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			info, err := entry.Info()
			if os.IsNotExist(err) {
				return nil
			}
			if err == nil && info.Size() > 0 {
				size += uint64(info.Size())
			}
			return err
		})
		if err != nil {
			return 0, err
		}
	}
	return size, ctx.Err()
}

// Retain resume data on interruption, but discard it once the file is verified.
func (f *Client) removeSegmentedFiles(t Task) {
	output := f.segmentedPath(t)
	for _, name := range []string{output, output + ".part", output + ".part.resume"} {
		_ = os.Remove(name)
	}
}
