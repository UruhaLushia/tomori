package launcher

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/loader"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) LoaderVersions(ctx context.Context, game, kind string) ([]loader.Version, error) {
	return loader.Versions(ctx, s.client, game, kind)
}

func (s *Service) InstallWithLoader(ctx context.Context, root string, v minecraft.VersionSummary, kind modpack.Loader, progress func(minecraft.Progress), onLine func(string)) (minecraft.Instance, error) {
	if kind.ID == "" {
		return s.Install(ctx, root, v, progress)
	}
	switch kind.ID {
	case "fabric", "quilt", "forge", "neoforge":
	default:
		return minecraft.Instance{}, fmt.Errorf(locales.UnsupportedLoader2(), kind.ID)
	}
	if err := fsutil.ValidateID(kind.Version); err != nil {
		return minecraft.Instance{}, err
	}
	parent, prism, err := minecraft.PrismInstallRoot(ctx, root)
	if err != nil {
		return minecraft.Instance{}, err
	}
	if !prism {
		parent = filepath.Join(root, "instances")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return minecraft.Instance{}, err
	}
	stage, err := os.MkdirTemp(parent, ".tomori-install-*")
	if err != nil {
		return minecraft.Instance{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	name := v.ID + " · " + kind.String()
	if prism {
		if _, err := s.installPrismStage(ctx, stage, name, v.ID, []modpack.Loader{kind}, progress); err != nil {
			return minecraft.Instance{}, err
		}
		return publishPrism(ctx, stage, parent)
	}
	base, err := minecraft.FetchVersion(ctx, s.client, v)
	if err != nil {
		return minecraft.Instance{}, err
	}
	gameRoot := filepath.Join(stage, "minecraft")
	installer := s.installer(gameRoot, progress)
	reuseRoots := s.installReuseRoots(root)
	installer.Isolate, installer.ReuseRoots = false, reuseRoots
	installer.ClientSources = []string{installer.ClientJar(base.ClientID())}
	installer.ClientSources = append(installer.ClientSources, installClientSources(base.ClientID(), reuseRoots)...)
	var javaPath string
	if kind.ID == "forge" || kind.ID == "neoforge" {
		if err := installer.Install(ctx, base); err != nil {
			return minecraft.Instance{}, err
		}
		j, err := java.Ensure(ctx, s.client, base, s.discoverJava(ctx), s.cfg.Java.RuntimePath(base.RequiredJava()), s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
		if err != nil {
			return minecraft.Instance{}, err
		}
		javaPath = j.Path
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "loader"})
	}
	li := loader.Installer{Fetcher: s.client, Root: gameRoot, Cache: s.cacheDir, Java: javaPath, ReuseRoots: reuseRoots, Progress: progress, OnLine: onLine}
	detail, err := li.Install(ctx, base, kind)
	if err != nil {
		return minecraft.Instance{}, err
	}
	if err := installer.Install(ctx, detail); err != nil {
		return minecraft.Instance{}, err
	}
	meta := minecraft.ManagedMetadata{Format: 1, Name: name, Version: detail.ID, GameVersion: v.ID, Loader: kind.String(), Loaders: []string{kind.ID}, ImportedAt: time.Now()}
	if err := minecraft.WriteManaged(stage, meta); err != nil {
		return minecraft.Instance{}, err
	}
	if _, err := minecraft.ReadManaged(stage); err != nil {
		return minecraft.Instance{}, err
	}
	if err := ctx.Err(); err != nil {
		return minecraft.Instance{}, err
	}
	target := filepath.Join(parent, "instance-"+rand.Text()[:16])
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return minecraft.Instance{}, errors.New(locales.TargetInstanceAlreadyExists())
	}
	if err := os.Rename(stage, target); err != nil {
		return minecraft.Instance{}, err
	}
	return minecraft.ReadManaged(target)
}
