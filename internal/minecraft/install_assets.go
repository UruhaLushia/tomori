package minecraft

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (i *Installer) assetBase() string {
	if i.AssetBase != "" {
		return i.AssetBase
	}
	return "https://resources.download.minecraft.net"
}

func (i *Installer) assetTasks(ctx context.Context, detail *VersionDetail) ([]download.Task, error) {
	if detail.AssetIndex.URL == "" {
		return nil, nil
	}
	indexPath := filepath.Join(i.AssetsDir(), "indexes", detail.AssetIndex.ID+".json")
	indexTask := download.Task{
		URL:  detail.AssetIndex.URL,
		Path: indexPath,
		SHA1: detail.AssetIndex.SHA1,
		Size: detail.AssetIndex.Size,
	}
	if err := i.Fetcher.FetchWithReuse(ctx, indexTask, i.reuseSources(nil)); err != nil {
		return nil, fmt.Errorf(locales.AssetIndexDownloadFailed(), err)
	}
	index, err := readAssetIndex(indexPath)
	if err != nil {
		return nil, err
	}
	tasks := make([]download.Task, 0, len(index.Objects))
	seen := make(map[string]bool, len(index.Objects))
	for _, obj := range index.Objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := obj.validate(); err != nil {
			return nil, err
		}
		if seen[obj.Hash] {
			continue
		}
		seen[obj.Hash] = true
		prefix := obj.Hash[:2]
		tasks = append(tasks, download.Task{
			URL:  i.assetBase() + "/" + prefix + "/" + obj.Hash,
			Path: filepath.Join(i.AssetsDir(), "objects", prefix, obj.Hash),
			SHA1: obj.Hash,
			Size: obj.Size,
		})
	}
	return tasks, nil
}

func (i *Installer) linkLegacyAssets(ctx context.Context, detail *VersionDetail) error {
	if detail.AssetIndex.URL == "" {
		return nil
	}
	release, err := i.Fetcher.FilePermit(ctx)
	if err != nil {
		return err
	}
	indexPath := filepath.Join(i.AssetsDir(), "indexes", detail.AssetIndex.ID+".json")
	index, err := readAssetIndex(indexPath)
	release()
	if err != nil {
		return err
	}
	if !index.Virtual && !index.MapToResources || len(index.Objects) == 0 {
		return nil
	}
	need, err := i.legacyAssetRequirement(ctx, detail, index)
	if err != nil {
		return err
	}
	space, err := storage.Assess(ctx, []storage.Requirement{need})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	base, prefix := i.legacyAssetLocation(detail, index)
	if err := os.MkdirAll(base, 0755); err != nil {
		return err
	}
	destination, err := os.OpenRoot(base)
	if err != nil {
		return err
	}
	defer destination.Close()
	source, err := os.OpenRoot(i.AssetsDir())
	if err != nil {
		return err
	}
	defer source.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type job struct {
		name   string
		object assetObject
	}
	jobs := make(chan job, 16)
	var workers sync.WaitGroup
	var fail sync.Once
	var firstErr error
	failed := func(err error) {
		fail.Do(func() { firstErr = err; cancel() })
	}
	for range min(len(index.Objects), max(1, min(i.Fetcher.Workers, 4))) {
		workers.Go(func() {
			for job := range jobs {
				if err := i.copyLegacyAsset(ctx, source, destination, filepath.Join(prefix, job.name), job.object); err != nil {
					failed(err)
					return
				}
			}
		})
	}
	for name, object := range index.Objects {
		name, err := legacyAssetName(name)
		if err != nil {
			failed(err)
			break
		}
		select {
		case jobs <- job{name, object}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	workers.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func (i *Installer) copyLegacyAsset(ctx context.Context, source, destination *os.Root, target string, object assetObject) error {
	unlock, err := i.Fetcher.Lock(ctx, "legacy-assets", filepath.Join(destination.Name(), target))
	if err != nil {
		return err
	}
	defer unlock()
	release, err := i.Fetcher.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	if fsutil.CompleteRootContext(ctx, destination, target, object.Hash, object.Size) {
		return nil
	}
	name := filepath.Join("objects", object.Hash[:2], object.Hash)
	return fsutil.CopyRootVerified(ctx, source, name, destination, target, object.Hash, object.Size)
}

type assetIndexFile struct {
	Objects        map[string]assetObject `json:"objects"`
	Virtual        bool                   `json:"virtual"`
	MapToResources bool                   `json:"map_to_resources"`
}

type assetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}

func assetSpaceTask(root string, object assetObject) download.Task {
	return download.Task{Path: filepath.Join(root, "objects", object.Hash[:2], object.Hash), Size: object.Size}
}

func (o assetObject) validate() error {
	if _, err := hex.DecodeString(o.Hash); err != nil || len(o.Hash) != 40 || o.Size < 0 {
		return fmt.Errorf(locales.InvalidAssetHashOrSizeAlt(), o.Hash)
	}
	return nil
}

func readAssetIndex(path string) (assetIndexFile, error) {
	var index assetIndexFile
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return index, err
	}
	defer root.Close()
	raw, err := fsutil.ReadRootFile(root, filepath.Base(path), 32<<20)
	if err != nil {
		return index, err
	}
	err = json.Unmarshal(raw, &index)
	return index, err
}
