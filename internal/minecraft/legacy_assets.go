package minecraft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (i *Installer) legacyAssetLocation(detail *VersionDetail, index assetIndexFile) (string, string) {
	if !index.MapToResources {
		return i.AssetsDir(), filepath.Join("virtual", detail.AssetIndex.ID)
	}
	base := i.GameDir
	if i.Isolate {
		base = i.VersionDir(detail.ID)
	}
	if i.ContentDir != "" {
		base = i.ContentDir
	}
	return base, "resources"
}

func legacyAssetName(name string) (string, error) {
	name, err := fsutil.SafePath(".", name)
	if err != nil {
		return "", err
	}
	for part := range strings.SplitSeq(filepath.ToSlash(name), "/") {
		if err := fsutil.ValidateID(part); err != nil {
			return "", err
		}
	}
	return name, nil
}

func (i *Installer) legacyAssetRequirement(ctx context.Context, detail *VersionDetail, index assetIndexFile) (storage.Requirement, error) {
	base, prefix := i.legacyAssetLocation(detail, index)
	need := storage.Requirement{Directory: filepath.Join(base, prefix)}
	release, err := i.Fetcher.FilePermit(ctx)
	if err != nil {
		return need, err
	}
	defer release()
	root, err := os.OpenRoot(base)
	if err != nil && !os.IsNotExist(err) {
		return need, err
	}
	if root != nil {
		defer root.Close()
	}
	seen := make(map[string]bool, len(index.Objects))
	for name, object := range index.Objects {
		if err := ctx.Err(); err != nil {
			return need, err
		}
		if err := object.validate(); err != nil {
			return need, err
		}
		name, err := legacyAssetName(name)
		if err != nil {
			return need, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return need, fmt.Errorf(locales.DownloadTargetConflict(), name)
		}
		seen[key] = true
		if root != nil {
			if info, err := root.Lstat(filepath.Join(prefix, name)); err == nil && info.Mode().IsRegular() && info.Size() == object.Size {
				continue
			}
		}
		need.Bytes = storage.Add(need.Bytes, uint64(object.Size))
	}
	return need, nil
}
