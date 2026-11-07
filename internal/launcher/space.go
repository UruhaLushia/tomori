package launcher

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/storage"
)

func (s *Service) installSpaceRequirements(ctx context.Context, directory string, summary minecraft.VersionSummary, kind modpack.Loader) ([]storage.Requirement, error) {
	parent, prism, err := minecraft.PrismInstallRoot(ctx, directory)
	if err != nil {
		return nil, err
	}
	if prism {
		var loaders []modpack.Loader
		if kind.ID != "" {
			loaders = append(loaders, kind)
		}
		return s.prismSpaceRequirements(ctx, parent, summary.ID, loaders)
	}
	detail, err := minecraft.FetchVersion(ctx, s.client, summary)
	if err != nil {
		return nil, err
	}
	root := directory
	if kind.ID != "" {
		parent = filepath.Join(directory, "instances")
		root = filepath.Join(parent, ".tomori-space-preview", "minecraft")
	}
	needs, err := s.installer(root, nil).SpaceRequirements(ctx, detail)
	if err != nil {
		return nil, err
	}
	if kind.ID != "" {
		needs = append(needs, storage.Requirement{Directory: directory, Unknown: true}, storage.Requirement{Directory: s.cacheDir, Unknown: true})
		if s.cfg.Java.AutoDownload && (kind.ID == "forge" || kind.ID == "neoforge") {
			needs = append(needs, storage.Requirement{Directory: s.runtimeDir, Unknown: true})
		}
	}
	return needs, nil
}

func (s *Service) prismSpaceRequirements(ctx context.Context, parent, game string, loaders []modpack.Loader) ([]storage.Requirement, error) {
	components, err := prismComponents(loaders)
	if err != nil {
		return nil, err
	}
	stage := filepath.Join(parent, ".tomori-space-preview")
	instance := minecraft.Instance{InstanceDir: stage, GameDir: filepath.Join(stage, "minecraft"), GameVersion: game, Format: minecraft.InstancePrism}
	pack := minecraft.PrismPack{FormatVersion: 1, Components: append([]minecraft.PrismComponent{{UID: "net.minecraft", Version: game}}, components...)}
	root, detail, err := s.preparePrismPack(ctx, instance, pack)
	if err != nil {
		return nil, err
	}
	needs, err := s.instanceInstaller(instance, root, nil).SpaceRequirements(ctx, detail)
	if err != nil {
		return nil, err
	}
	if s.cfg.Java.AutoDownload {
		needs = append(needs, storage.Requirement{Directory: s.runtimeDir, Unknown: true})
	}
	return needs, nil
}

func (s *Service) AssessInstall(ctx context.Context, directory string, summary minecraft.VersionSummary, kind modpack.Loader) (storage.Report, error) {
	needs, err := s.installSpaceRequirements(ctx, directory, summary, kind)
	if err != nil {
		return storage.Report{}, err
	}
	return storage.Assess(ctx, needs)
}

func (s *Service) packSpaceRequirements(ctx context.Context, directory string, info modpack.Info, optional bool) ([]storage.Requirement, error) {
	parent, prism, err := minecraft.PrismInstallRoot(ctx, directory)
	if err != nil {
		return nil, err
	}
	if !prism {
		parent = filepath.Join(directory, "instances")
	}
	return s.packContentSpaceRequirements(ctx, parent, info, optional)
}

func (s *Service) packContentSpaceRequirements(ctx context.Context, directory string, info modpack.Info, optional bool) ([]storage.Requirement, error) {
	needs := []storage.Requirement{{Directory: directory, Bytes: info.ExpandedSize}}
	var tasks []download.Task
	var installed uint64
	var unknown bool
	for _, f := range info.Files {
		if f.Optional && !optional {
			continue
		}
		installed = storage.Add(installed, uint64(max(0, f.Size)))
		unknown = unknown || f.Size <= 0
		tasks = append(tasks, download.Task{Path: cache.File(s.cacheDir, strings.ToLower(f.SHA1)), Size: f.Size})
	}
	cacheNeeds, err := s.client.SpaceRequirements(ctx, tasks)
	if err != nil {
		return nil, err
	}
	needs = append(needs, cacheNeeds...)
	needs = append(needs, storage.Requirement{Directory: directory, Bytes: installed, Unknown: unknown})
	return needs, nil
}

func (s *Service) AssessPackImport(ctx context.Context, directory string, info modpack.Info, optional bool) (storage.Report, error) {
	needs, err := s.packSpaceRequirements(ctx, directory, info, optional)
	if err != nil {
		return storage.Report{}, err
	}
	parent, prism, err := minecraft.PrismInstallRoot(ctx, directory)
	if err != nil {
		return storage.Report{}, err
	}
	if prism {
		runtime, err := s.prismSpaceRequirements(ctx, parent, info.GameVersion, info.Loaders)
		if err != nil {
			return storage.Report{}, err
		}
		return storage.Assess(ctx, append(needs, runtime...))
	}
	manifest, err := minecraft.FetchManifest(ctx, s.client)
	if err != nil {
		return storage.Report{}, err
	}
	for _, summary := range manifest.Versions {
		if summary.ID != info.GameVersion {
			continue
		}
		parent = filepath.Join(directory, "instances")
		detail, err := minecraft.FetchVersion(ctx, s.client, summary)
		if err != nil {
			return storage.Report{}, err
		}
		runtime, err := s.installer(filepath.Join(parent, ".tomori-space-preview", "minecraft"), nil).SpaceRequirements(ctx, detail)
		if err != nil {
			return storage.Report{}, err
		}
		needs = append(needs, runtime...)
		break
	}
	needs = append(needs, storage.Requirement{Directory: directory, Unknown: true})
	if len(info.Loaders) > 0 {
		needs = append(needs, storage.Requirement{Directory: s.cacheDir, Unknown: true})
		if s.cfg.Java.AutoDownload {
			needs = append(needs, storage.Requirement{Directory: s.runtimeDir, Unknown: true})
		}
	}
	return storage.Assess(ctx, needs)
}

func (s *Service) AssessRemotePackImport(ctx context.Context, directory string, preview PackPreview, optional bool) (storage.Report, error) {
	version := preview.Version
	cache, err := s.client.SpaceRequirements(ctx, []download.Task{{Path: s.packCachePath(version), Size: version.Size}})
	if err != nil {
		return storage.Report{}, err
	}
	if !preview.Deferred {
		content, err := s.packSpaceRequirements(ctx, directory, preview.Info, optional)
		if err != nil {
			return storage.Report{}, err
		}
		cache = append(cache, content...)
	}
	return storage.Assess(ctx, append(cache, storage.Requirement{Directory: directory, Unknown: true}))
}

func (s *Service) AssessPackUpdateTarget(ctx context.Context, instance minecraft.Instance, info modpack.Info, optional bool, preview *PackPreview) (storage.Report, error) {
	s = s.forInstance(instance)
	needs, err := s.packContentSpaceRequirements(ctx, instance.InstanceDir, info, optional)
	if err != nil {
		return storage.Report{}, err
	}
	needs = append(needs, storage.Requirement{Directory: instance.ContentDirectory(s.cfg.Game.Isolate), Unknown: true})
	if preview != nil {
		v := preview.Version
		cache, err := s.client.SpaceRequirements(ctx, []download.Task{{Path: s.packCachePath(v), Size: v.Size}})
		if err != nil {
			return storage.Report{}, err
		}
		needs = append(needs, cache...)
	}
	return storage.Assess(ctx, needs)
}

func (s *Service) AssessResourceUpdates(ctx context.Context, instance minecraft.Instance, kind resource.Kind, items []ResourceUpdate) (storage.Report, error) {
	s = s.forInstance(instance)
	root, err := resource.Directory(instance.ContentDirectory(s.cfg.Game.Isolate), kind)
	if err != nil {
		return storage.Report{}, err
	}
	return s.assessResourceUpdates(ctx, root, items)
}

func (s *Service) AssessWorldPackUpdates(ctx context.Context, instance minecraft.Instance, folder string, items []ResourceUpdate) (storage.Report, error) {
	if err := fsutil.ValidateID(folder); err != nil {
		return storage.Report{}, err
	}
	root := filepath.Join(instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), "saves", folder, "datapacks")
	return s.assessResourceUpdates(ctx, root, items)
}

func (s *Service) assessResourceUpdates(ctx context.Context, root string, items []ResourceUpdate) (storage.Report, error) {
	var installed, cache uint64
	for _, item := range items {
		next := uint64(max(0, item.Size))
		installed = storage.Add(installed, storage.Add(next, next))
		installed = storage.Add(installed, uint64(max(0, item.Entry.Size)))
		cache = storage.Add(cache, next)
		if s.client.Segmented && s.client.Connections > 1 && item.Size >= s.client.SegmentThreshold {
			cache = storage.Add(cache, next)
		}
	}
	return storage.Assess(ctx, []storage.Requirement{{Directory: root, Bytes: installed, Unknown: true}, {Directory: s.cacheDir, Bytes: cache, Unknown: true}})
}
