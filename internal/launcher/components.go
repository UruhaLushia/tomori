package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/loader"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) ChangeComponents(ctx context.Context, instance minecraft.Instance, expected [32]byte, game string, kind modpack.Loader, progress func(minecraft.Progress), onLine func(string)) error {
	s = s.forInstance(instance)
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return err
	}
	defer release()
	return s.prepareComponents(ctx, instance, expected, game, kind, nil, progress, onLine, func(detail *minecraft.VersionDetail, raw []byte) error {
		if detail == nil && raw == nil {
			return nil
		}
		return minecraft.CommitComponents(ctx, instance, expected, detail, raw)
	})
}

func (s *Service) prepareComponents(ctx context.Context, instance minecraft.Instance, expected [32]byte, game string, kind modpack.Loader, profile *minecraft.VersionDetail, progress func(minecraft.Progress), onLine func(string), ready func(*minecraft.VersionDetail, []byte) error) error {
	current, err := minecraft.InspectComponents(ctx, instance)
	if err != nil {
		return err
	}
	if current.Fingerprint != expected {
		return errors.New(locales.InstanceComponentsChangedReopenThisDialogToEditThem())
	}
	if current.Reason != "" {
		return errors.New(current.Reason)
	}
	if err := fsutil.ValidateID(game); err != nil {
		return err
	}
	if kind.ID != "" {
		versions, err := s.LoaderVersions(ctx, game, kind.ID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(versions, func(v loader.Version) bool { return v.ID == kind.Version }) {
			return errors.New(locales.TheLoaderIsIncompatibleWithTheSelectedGameVersion())
		}
	}
	if profile == nil && current.Game == game && current.Loader == kind.ID && (kind.ID == "" || current.LoaderVersion == kind.Version) {
		return ready(nil, nil)
	}
	if instance.Format == minecraft.InstancePrism {
		pack, raw, err := minecraft.UpdatedPrismComponents(ctx, instance, game, kind.ID, kind.Version)
		if err != nil {
			return err
		}
		root, detail, err := s.preparePrismPack(ctx, instance, pack)
		if err != nil {
			return err
		}
		j, err := java.Ensure(ctx, s.client, detail, s.discoverJava(ctx), s.cfg.Java.RuntimePath(detail.RequiredJava()), s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
		if err != nil {
			return err
		}
		instance.GameVersion = game
		installer := s.instanceInstaller(instance, root, progress)
		if j.Arch != "" {
			installer.Env.Arch = j.Arch
		}
		if err := installer.Install(ctx, detail); err != nil {
			return err
		}
		return ready(nil, raw)
	}
	manifest, err := s.Catalog(ctx)
	if manifest == nil {
		return err
	}
	index := slices.IndexFunc(manifest.Versions, func(v minecraft.VersionSummary) bool { return v.ID == game })
	if index < 0 {
		return fmt.Errorf(locales.MinecraftNotFound(), game)
	}
	base, err := minecraft.FetchVersion(ctx, s.client, manifest.Versions[index])
	if err != nil {
		return err
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "java-check"})
	}
	j, err := java.Ensure(ctx, s.client, base, s.discoverJava(ctx), s.cfg.Java.RuntimePath(base.RequiredJava()), s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Join(instance.GameDir, "versions"), ".tomori-components-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	installer := s.installer(instance.GameDir, progress)
	if j.Arch != "" {
		installer.Env.Arch = j.Arch
	}
	installer.Isolate, installer.ContentDir = true, stage
	installer.ReuseRoots = s.installReuseRoots(instance.GameDir, configGameRoot(instance))
	if current.Game == game {
		installer.ClientSources = []string{filepath.Join(instance.GameDir, "versions", instance.ID, instance.ID+".jar")}
	}
	installer.ClientSources = append(installer.ClientSources, installClientSources(base.ClientID(), installer.ReuseRoots)...)
	stageID := filepath.Base(stage)
	detail := base
	if kind.ID != "" {
		if kind.ID == "forge" || kind.ID == "neoforge" {
			stagedBase := *base
			stagedBase.ID, stagedBase.Jar = stageID, game
			if err := installer.Install(ctx, &stagedBase); err != nil {
				return err
			}
		}
		if progress != nil {
			progress(minecraft.Progress{Stage: "loader"})
		}
		li := loader.Installer{Fetcher: s.client, Root: instance.GameDir, Cache: s.cacheDir, Java: j.Path, ReuseRoots: installer.ReuseRoots, Progress: progress, OnLine: onLine}
		detail, err = li.Install(ctx, base, kind)
		if err != nil {
			return err
		}
	} else if profile != nil {
		detail = minecraft.MergeVersion(base, profile)
	}
	detail.ID, detail.Jar, detail.MinecraftVersion = stageID, game, game
	for index, arg := range detail.Arguments.JVM {
		var values minecraft.ArgumentValue
		for n, value := range arg.Value {
			if strings.Contains(value, "${version_name}.jar") {
				if values == nil {
					values = slices.Clone(arg.Value)
					detail.Arguments.JVM[index].Value = values
				}
				values[n] = strings.ReplaceAll(value, "${version_name}.jar", game+".jar")
			}
		}
	}
	if err := installer.Install(ctx, detail); err != nil {
		return err
	}
	detail.ID = instance.ID
	return ready(detail, nil)
}

func configGameRoot(instance minecraft.Instance) string {
	if instance.SourceDir != "" {
		return instance.SourceDir
	}
	return instance.GameDir
}
