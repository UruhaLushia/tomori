package minecraft

import (
	"context"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/storage"
)

func (i *Installer) SpaceRequirements(ctx context.Context, detail *VersionDetail) ([]storage.Requirement, error) {
	if err := detail.Validate(); err != nil {
		return nil, err
	}
	libs, natives, err := i.libraryTasks(detail)
	if err != nil {
		return nil, err
	}
	tasks := append(i.clientTasks(detail), libs...)
	tasks = append(tasks, natives...)
	tasks = append(tasks, i.loggingTasks(detail)...)
	needs, err := i.Fetcher.SpaceRequirements(ctx, tasks)
	if err != nil {
		return nil, err
	}
	if detail.AssetIndex.URL != "" {
		index, readErr := readAssetIndex(filepath.Join(i.AssetsDir(), "indexes", detail.AssetIndex.ID+".json"))
		if readErr == nil {
			assets := tasks[:0]
			for _, object := range index.Objects {
				if err := object.validate(); err != nil {
					return nil, err
				}
				assets = append(assets, assetSpaceTask(i.AssetsDir(), object))
			}
			assetNeeds, err := i.Fetcher.SpaceRequirements(ctx, assets)
			if err != nil {
				return nil, err
			}
			needs = append(needs, assetNeeds...)
			if index.Virtual || index.MapToResources {
				legacy, err := i.legacyAssetRequirement(ctx, detail, index)
				if err != nil {
					return nil, err
				}
				needs = append(needs, legacy)
			}
		} else {
			needs = append(needs, storage.Requirement{Directory: i.AssetsDir(), Bytes: uint64(max(0, detail.AssetIndex.TotalSize)), Unknown: true})
		}
	}
	needs = append(needs, storage.Requirement{Directory: filepath.Dir(i.NativesDir(detail.ID)), Unknown: len(natives) > 0})
	return needs, nil
}
