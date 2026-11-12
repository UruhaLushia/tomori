package launcher

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) ImportLocalResources(ctx context.Context, instance minecraft.Instance, files []resource.LocalFile, world string, progress func(minecraft.Progress)) (int, error) {
	if !instance.CanLaunch() || len(files) == 0 || len(files) > 1000 {
		return 0, errors.New(locales.InvalidTargetInstance())
	}
	root := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	groups := make(map[resource.Kind][]string)
	for _, file := range files {
		if !file.Kind.Valid() {
			return 0, errors.New(locales.UnsupportedFileType())
		}
		if file.Kind == resource.DataPack && world == "" {
			return 0, errors.New(locales.ManageDataPacksWithinAWorld())
		}
		groups[file.Kind] = append(groups[file.Kind], file.Path)
	}
	count := 0
	var problems []error
	// Stable order also keeps errors and partial-batch results predictable.
	for _, kind := range []resource.Kind{resource.Mod, resource.ResourcePack, resource.ShaderPack, resource.World, resource.DataPack} {
		paths := groups[kind]
		if len(paths) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return count, errors.Join(append(problems, err)...)
		}
		switch kind {
		case resource.World:
			for _, path := range paths {
				preview, err := s.InspectWorld(ctx, path)
				if err == nil {
					name := filepath.Base(path)
					if !preview.Directory {
						name = name[:len(name)-len(filepath.Ext(name))]
					}
					_, err = s.ImportWorld(ctx, instance, preview, name, progress)
				}
				if err != nil {
					problems = append(problems, fmt.Errorf("%s: %w", filepath.Base(path), err))
				} else {
					count++
				}
			}
		case resource.DataPack:
			n, err := s.ImportWorldPacks(ctx, instance, world, paths)
			count += n
			if err != nil {
				problems = append(problems, err)
			}
		default:
			release, err := s.lockRoot(ctx, root, progress)
			if err != nil {
				problems = append(problems, err)
				continue
			}
			releaseFile, err := s.client.FilePermit(ctx)
			if err == nil {
				var n int
				n, err = resource.Import(ctx, root, kind, paths, resource.Compatibility{GameVersion: instance.GameVersion, Loader: instance.Loader, Loaders: instance.Loaders})
				count += n
				releaseFile()
			}
			release()
			if err != nil {
				problems = append(problems, err)
			}
		}
	}
	return count, errors.Join(problems...)
}
