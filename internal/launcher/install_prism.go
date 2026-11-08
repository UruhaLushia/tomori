package launcher

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

func prismComponents(loaders []modpack.Loader) ([]minecraft.PrismComponent, error) {
	var components []minecraft.PrismComponent
	for _, loader := range loaders {
		uid := minecraft.LoaderUID(loader.ID)
		if uid == "" {
			return nil, fmt.Errorf(locales.CannotInstallLoaderInPrism(), loader.ID)
		}
		components = append(components, minecraft.PrismComponent{UID: uid, Version: loader.Version})
	}
	return components, nil
}

func publishPrism(ctx context.Context, stage, parent string) (minecraft.Instance, error) {
	if _, err := minecraft.ReadPrismInstance(stage); err != nil {
		return minecraft.Instance{}, err
	}
	if err := ctx.Err(); err != nil {
		return minecraft.Instance{}, err
	}
	target := filepath.Join(parent, "tomori-"+rand.Text()[:16])
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return minecraft.Instance{}, errors.New(locales.TargetInstanceAlreadyExists())
	}
	if err := os.Rename(stage, target); err != nil {
		return minecraft.Instance{}, err
	}
	if resolved, err := filepath.EvalSymlinks(target); err == nil {
		target = resolved
	}
	return minecraft.ReadPrismInstance(target)
}

func (s *Service) installPrism(ctx context.Context, parent string, v minecraft.VersionSummary, progress func(minecraft.Progress)) (minecraft.Instance, error) {
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return minecraft.Instance{}, err
	}
	stage, err := os.MkdirTemp(parent, ".tomori-install-*")
	if err != nil {
		return minecraft.Instance{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	if _, err := s.installPrismStage(ctx, stage, v.ID, v.ID, nil, nil, nil, progress); err != nil {
		return minecraft.Instance{}, err
	}
	return publishPrism(ctx, stage, parent)
}

func (s *Service) installPrismStage(ctx context.Context, stage, name, game string, loaders []modpack.Loader, prismPack []minecraft.PrismComponent, patches map[string][]byte, progress func(minecraft.Progress)) (*minecraft.VersionDetail, error) {
	components := prismPack
	if len(components) == 0 {
		var err error
		components, err = prismComponents(loaders)
		if err != nil {
			return nil, err
		}
	}
	if err := minecraft.WritePrismInstance(stage, name, game, components); err != nil {
		return nil, err
	}
	for name, raw := range patches {
		if filepath.Base(name) != name || !strings.HasSuffix(strings.ToLower(name), ".json") {
			return nil, fmt.Errorf(locales.InvalidModpackPath(), filepath.Join("patches", name))
		}
		if _, err := fsutil.SafePath("patches", name); err != nil {
			return nil, err
		}
		if err := fsutil.WriteAtomic(filepath.Join(stage, "patches", name), raw); err != nil {
			return nil, err
		}
	}
	instance, err := minecraft.ReadPrismInstance(stage)
	if err != nil {
		return nil, err
	}
	root, detail, err := s.preparePrism(ctx, instance)
	if err != nil {
		return nil, err
	}
	j, err := java.Ensure(ctx, s.client, s.http, detail, s.discoverJava(ctx), s.cfg.Java.RuntimePath(detail.RequiredJava()), s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
	if err != nil {
		return nil, err
	}
	installer := s.instanceInstaller(instance, root, progress)
	if j.Arch != "" {
		installer.Env.Arch = j.Arch
	}
	return detail, installer.Install(ctx, detail)
}
