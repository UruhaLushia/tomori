package launcher

import (
	"context"
	"errors"
	"sync"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type ResourceUpdateCheck = resourcecatalog.UpdateCheck

func (s *Service) CheckResourceUpdates(ctx context.Context, instance minecraft.Instance, kind resource.Kind, source resourcecatalog.Source) (ResourceUpdateCheck, error) {
	var result ResourceUpdateCheck
	if !kind.Valid() || !source.Valid() {
		return result, errors.New(locales.InvalidResourceSourceOrType())
	}
	if kind == resource.DataPack {
		return result, errors.New(locales.ManageDataPacksWithinAWorld())
	}
	s = s.forInstance(instance)
	root := instance.ContentDirectory(s.cfg.Game.Isolate)
	entries, err := resource.Scan(ctx, root, kind)
	if err != nil {
		return result, err
	}
	return s.checkResourceUpdates(ctx, instance, root, kind, source, entries)
}

func (s *Service) CheckWorldPackUpdates(ctx context.Context, instance minecraft.Instance, folder string, source resourcecatalog.Source) (ResourceUpdateCheck, error) {
	w, closeWorld, err := s.openWorldResources(ctx, instance, folder, nil)
	if err != nil {
		return ResourceUpdateCheck{}, err
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		closeWorld()
		return ResourceUpdateCheck{}, err
	}
	packs, err := instancecontent.ReadWorldPacks(ctx, w.root)
	var entries []resource.Entry
	if err == nil {
		entries, err = resource.Scan(ctx, w.path, resource.DataPack)
	}
	release()
	root := w.path
	closeWorld()
	if err != nil {
		return ResourceUpdateCheck{}, err
	}
	if packs.Version != "" {
		instance.GameVersion = packs.Version
	}
	for i := range entries {
		entries[i].Enabled = packs.IsEnabled(entries[i].File)
	}
	return s.checkResourceUpdates(ctx, instance, root, resource.DataPack, source, entries)
}

func (s *Service) checkResourceUpdates(ctx context.Context, instance minecraft.Instance, root string, kind resource.Kind, source resourcecatalog.Source, entries []resource.Entry) (ResourceUpdateCheck, error) {
	var result ResourceUpdateCheck
	if !source.Valid() {
		return result, errors.New(locales.InvalidResourceSourceOrType())
	}
	files := make([]resource.Entry, 0, len(entries))
	for _, entry := range entries {
		if !entry.Directory {
			files = append(files, entry)
		}
	}
	if len(files) > 1000 {
		return result, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	result.Checked = len(files)
	if len(files) == 0 {
		return result, nil
	}
	hashes := make([]string, len(files))
	fingerprints := make([]resourcecatalog.Fingerprint, len(files))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	next := 0
	var problem error
	var workers sync.WaitGroup
	for range min(4, len(files)) {
		workers.Go(func() {
			for {
				mu.Lock()
				if next == len(files) || ctx.Err() != nil {
					mu.Unlock()
					return
				}
				i := next
				next++
				mu.Unlock()
				hash, fp, err := s.resourceHash(ctx, root, kind, files[i], source == resourcecatalog.CurseForge)
				if err != nil {
					mu.Lock()
					if problem == nil {
						problem = err
						cancel()
					}
					mu.Unlock()
					return
				}
				hashes[i], fingerprints[i] = hash, fp
			}
		})
	}
	workers.Wait()
	if problem != nil {
		return result, problem
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	candidates, err := (resourcecatalog.Client{Fetcher: s.http, CacheDir: s.cacheDir, Lock: s.client.Lock}).Updates(ctx, source, resourceFilter(instance, kind, ""), hashes, fingerprints)
	if err != nil {
		return result, err
	}
	counts := make(map[string]int)
	for _, hash := range hashes {
		if origin, ok := candidates.Known[hash]; ok {
			counts[origin.ProjectID]++
		} else {
			result.Unknown++
		}
	}
	for i, hash := range hashes {
		origin, known := candidates.Known[hash]
		if !known {
			continue
		}
		if counts[origin.ProjectID] > 1 || candidates.Duplicates[origin.ProjectID] {
			result.Conflicts++
			continue
		}
		candidate, ok := candidates.Updates[hash]
		if ok && candidate.Size == files[i].Size {
			result.Updates = append(result.Updates, ResourceUpdate{Entry: files[i], SHA1: hash, Origin: candidate.Origin, Version: candidate.Version, Size: candidate.NextSize})
		}
	}
	return result, ctx.Err()
}
