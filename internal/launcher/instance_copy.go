package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type InstanceCopy struct {
	Name, Directory                     string
	Worlds, Screenshots, Logs, Personal bool
}

func (s *Service) CopyInstance(ctx context.Context, instance minecraft.Instance, request InstanceCopy, progress func(minecraft.Progress)) (out minecraft.Instance, err error) {
	var empty minecraft.Instance
	name := strings.TrimSpace(request.Name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 || strings.ContainsFunc(name, unicode.IsControl) {
		return empty, errors.New(locales.Use1128CharactersWithoutControlCharacters())
	}
	allowed := false
	for _, directory := range s.cfg.Directories.Entries {
		if directory.Path == request.Directory {
			allowed = true
			break
		}
	}
	if !allowed {
		return empty, errors.New(locales.InstallLocationHasNotBeenAdded())
	}
	s = s.forInstance(instance)
	content := instance.ContentDirectory(s.cfg.Game.Isolate)
	release, err := s.lockRoot(ctx, content, progress)
	if err != nil {
		return empty, err
	}
	defer release()
	components, err := minecraft.InspectComponents(ctx, instance)
	if err != nil {
		return empty, err
	}
	parent, prism, err := minecraft.PrismInstallRoot(ctx, request.Directory)
	if err != nil {
		return empty, err
	}
	if request.Directory == config.DefaultGameDirectory() {
		prism = false
	}
	if !prism {
		parent = filepath.Join(request.Directory, "instances")
	}
	if err := os.MkdirAll(parent, 0755); err != nil {
		return empty, err
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return empty, err
	}
	for _, source := range []string{content, instance.InstanceDir} {
		real, err := filepath.EvalSymlinks(source)
		if err != nil {
			return empty, err
		}
		if instance.Format == minecraft.InstanceMinecraft && !s.cfg.Game.Isolate && real == filepath.Clean(instance.GameDir) && parent == filepath.Join(real, "instances") {
			continue
		}
		if pathWithin(real, parent) {
			return empty, errors.New(locales.TheCopyDestinationCannotBeInsideTheSourceInstance())
		}
	}
	if prism && instance.Format != minecraft.InstancePrism && components.Reason != "" {
		return empty, fmt.Errorf(locales.CannotConvertThisInstanceToPrism(), components.Reason)
	}
	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		return empty, err
	}
	defer parentRoot.Close()
	stage, err := newCopyDirectory(parentRoot, parent, ".tomori-copy-")
	if err != nil {
		return empty, err
	}
	stageInfo, err := parentRoot.Lstat(filepath.Base(stage))
	if err != nil {
		return empty, err
	}
	defer func() {
		current, statErr := parentRoot.Lstat(filepath.Base(stage))
		if statErr == nil && os.SameFile(stageInfo, current) {
			err = errors.Join(err, parentRoot.RemoveAll(filepath.Base(stage)))
		}
	}()
	stageRoot, err := parentRoot.OpenRoot(filepath.Base(stage))
	if err != nil {
		return empty, err
	}
	defer stageRoot.Close()
	if err := stageRoot.Mkdir("minecraft", 0755); err != nil {
		return empty, err
	}
	destination, err := stageRoot.OpenRoot("minecraft")
	if err != nil {
		return empty, err
	}
	defer destination.Close()
	sourcePath := content
	if instance.Format != minecraft.InstanceMinecraft {
		sourcePath = instance.GameDir
	}
	source, err := os.OpenRoot(sourcePath)
	if err != nil {
		return empty, err
	}
	defer source.Close()
	var legacyRoot *os.Root
	if instance.Format == minecraft.InstanceMinecraft {
		legacyRoot, err = os.OpenRoot(instance.GameDir)
		if err != nil {
			return empty, err
		}
		defer legacyRoot.Close()
	}
	copyProgress := func(p fsutil.CopyProgress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "copy", Done: p.Bytes, Total: p.TotalBytes, Files: p.TotalFiles, FilesDone: p.Files})
		}
	}
	copier := fsutil.NewTreeCopier(ctx, destination, s.client.FilePermit, copyProgress)
	defer copier.Close()
	err = copier.Tree(source, func(path string, directory bool) bool { return copyContent(path, instance, request) })
	if err == nil && instance.Format == minecraft.InstanceMinecraft {
		err = s.copyLegacyFiles(ctx, copier, legacyRoot, destination, instance)
	}
	if err != nil {
		copier.Cancel()
	}
	if closeErr := copier.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return empty, err
	}
	if instance.Format == minecraft.InstancePrism {
		if err := s.copyPrismMetadata(ctx, instance, stageRoot, name); err != nil {
			return empty, err
		}
	} else {
		if prism {
			var loaders []minecraft.PrismComponent
			if components.Loader != "" {
				loaders = append(loaders, minecraft.PrismComponent{UID: minecraft.LoaderUID(components.Loader), Version: components.LoaderVersion})
			}
			pack := minecraft.PrismPack{FormatVersion: 1, Components: append([]minecraft.PrismComponent{{UID: "net.minecraft", Version: components.Game}}, loaders...)}
			if err := writeCopyJSON(ctx, stageRoot, "mmc-pack.json", pack); err != nil {
				return empty, err
			}
			if err := writeCopyPrismConfig(ctx, stageRoot, name); err != nil {
				return empty, err
			}
			if err := writeCopyJSON(ctx, stageRoot, minecraft.PrismMetadataFile, minecraft.PrismMetadata{Description: instance.Description, PackFormat: instance.PackFormat, PackVersion: instance.PackVersion, PackSource: instance.PackSource}); err != nil {
				return empty, err
			}
		} else {
			meta := minecraft.ManagedMetadata{Format: 1, Name: name, Description: instance.Description, Version: instance.ID, GameVersion: instance.GameVersion, Loader: instance.Loader, Loaders: instance.Loaders, PackFormat: instance.PackFormat, PackVersion: instance.PackVersion, PackSource: instance.PackSource, ImportedAt: time.Now()}
			if err := writeCopyJSON(ctx, stageRoot, minecraft.InstanceMetadata, meta); err != nil {
				return empty, err
			}
		}
	}
	if err := copyPackOrigin(ctx, instance, stageRoot); err != nil {
		return empty, err
	}
	if icon, iconErr := minecraft.LoadInstanceIcon(ctx, instance); iconErr == nil && icon != nil {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, icon); err != nil {
			return empty, err
		}
		if err := fsutil.WriteRootAtomic(ctx, stageRoot, ".tomori-icon.png", encoded.Bytes()); err != nil {
			return empty, err
		}
	}
	after, err := minecraft.InspectComponents(ctx, instance)
	if err != nil {
		return empty, err
	}
	if after.Fingerprint != components.Fingerprint || after.ProfileFingerprint != components.ProfileFingerprint {
		return empty, errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	read := minecraft.ReadManaged
	marker := minecraft.InstanceMetadata
	if prism || instance.Format == minecraft.InstancePrism {
		read, marker = minecraft.ReadPrismInstance, "mmc-pack.json"
	}
	if _, err := read(stage); err != nil {
		return empty, err
	}
	origins, err := resource.LoadOrigins(s.store.Dir(), content)
	if err != nil {
		return empty, err
	}
	if err := ctx.Err(); err != nil {
		return empty, err
	}
	target, cleanup, err := publishCopy(ctx, parentRoot, parent, stage, marker, "instance-")
	if err != nil {
		return empty, err
	}
	keep := false
	defer func() {
		if !keep {
			err = errors.Join(err, cleanup())
		}
	}()
	result, err := read(target)
	if err != nil {
		return empty, err
	}
	if len(origins.Files) > 0 {
		if err := origins.Save(s.store.Dir(), result.GameDir); err != nil {
			return empty, err
		}
	}
	result.SourceDir, result.SourceName = request.Directory, "Tomori"
	keep = true
	return result, nil
}

func copyPackOrigin(ctx context.Context, instance minecraft.Instance, destination *os.Root) error {
	root, err := os.OpenRoot(instance.InstanceDir)
	if err != nil {
		return err
	}
	defer root.Close()
	origin, err := modpack.ReadOrigin(ctx, root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return origin.Write(ctx, destination)
}

func pathWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func copyContent(path string, instance minecraft.Instance, request InstanceCopy) bool {
	path = filepath.ToSlash(path)
	base := strings.ToLower(filepath.Base(path))
	if strings.HasPrefix(base, ".tomori-") || base == "session.lock" || base == ".git" || base == ".svn" {
		return false
	}
	switch base {
	case "accounts.json", "launcher_accounts.json", "launcher_profiles.json", "launcher_profiles_microsoft_store.json", "usercache.json", "usernamecache.json", "authlib-injector.json", "hmcl.json", "hmcl_profiles.json", "config.db", "config.db-wal", "config.db-shm":
		return false
	}
	for part := range strings.SplitSeq(strings.ToLower(path), "/") {
		switch part {
		case "saves", "backups":
			if !request.Worlds {
				return false
			}
		case "screenshots":
			if !request.Screenshots {
				return false
			}
		case "logs", "crash-reports", ".mixin.out":
			if !request.Logs {
				return false
			}
		case "natives", "natives-linux", "natives-windows", "natives-osx", ".cache":
			return false
		}
	}
	top, _, _ := strings.Cut(strings.ToLower(path), "/")
	switch top {
	case "saves", "backups":
		return request.Worlds
	case "screenshots":
		return request.Screenshots
	case "logs", "crash-reports", ".mixin.out":
		return request.Logs
	case "natives", "natives-linux", "natives-windows", "natives-osx", ".cache", "instances":
		return false
	case "versions", "libraries", "assets":
		return instance.Format != minecraft.InstanceMinecraft
	case "mods", "coremods", "config", "defaultconfigs", "resourcepacks", "shaderpacks", "resources", "kubejs", "scripts", "crafttweaker", "global_packs", "openloader", "patchouli_books", "icon.png", "pack.png":
		return true
	case "pcl", "hmcl", "launcher_libraries", "launcher_assets":
		return false
	}
	if instance.Format == minecraft.InstanceMinecraft && (path == instance.ID+".json" || path == instance.ID+".jar") {
		return false
	}
	return request.Personal
}

func writeCopyJSON(ctx context.Context, root *os.Root, name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteRootAtomic(ctx, root, name, raw)
}

func (s *Service) copyPrismMetadata(ctx context.Context, instance minecraft.Instance, dst *os.Root, name string) error {
	root, err := os.OpenRoot(instance.InstanceDir)
	if err != nil {
		return err
	}
	defer root.Close()
	copier := fsutil.NewTreeCopier(ctx, dst, s.client.FilePermit, nil)
	defer copier.Close()
	err = copier.Tree(root, func(path string, _ bool) bool {
		top, _, _ := strings.Cut(filepath.ToSlash(path), "/")
		return top == "patches" || top == "mmc-pack.json" || top == minecraft.PrismMetadataFile
	})
	if closeErr := copier.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return writeCopyPrismConfig(ctx, dst, name)
}

func writeCopyPrismConfig(ctx context.Context, root *os.Root, name string) error {
	return fsutil.WriteRootAtomic(ctx, root, "instance.cfg", []byte("[General]\nInstanceType=OneSix\nname="+strconv.Quote(name)+"\niconKey=default\n"))
}

func newCopyDirectory(parent *os.Root, directory, prefix string) (string, error) {
	for range 10 {
		name := prefix + rand.Text()[:16]
		err := parent.Mkdir(name, 0700)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return filepath.Join(directory, name), nil
	}
	return "", errors.New(locales.TargetInstanceAlreadyExists())
}

func publishCopy(ctx context.Context, parent *os.Root, directory, stage, marker, prefix string) (string, func() error, error) {
	target, err := newCopyDirectory(parent, directory, prefix)
	if err != nil {
		return "", nil, err
	}
	name := filepath.Base(target)
	info, err := parent.Lstat(name)
	if err != nil {
		return "", nil, err
	}
	cleanup := func() error {
		current, err := parent.Lstat(name)
		if err != nil {
			return err
		}
		if !os.SameFile(info, current) {
			return errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
		}
		return parent.RemoveAll(name)
	}
	stageRoot, err := parent.OpenRoot(filepath.Base(stage))
	if err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	defer stageRoot.Close()
	stageDir, err := stageRoot.Open(".")
	if err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	defer stageDir.Close()
	for {
		entries, readErr := stageDir.ReadDir(128)
		for _, entry := range entries {
			if entry.Name() == marker {
				continue
			}
			if err = ctx.Err(); err != nil {
				break
			}
			if err = parent.Rename(filepath.Join(filepath.Base(stage), entry.Name()), filepath.Join(name, entry.Name())); err != nil {
				break
			}
		}
		if err != nil || errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = parent.Rename(filepath.Join(filepath.Base(stage), marker), filepath.Join(name, marker))
	}
	if err != nil {
		return "", nil, errors.Join(err, cleanup())
	}
	return target, cleanup, nil
}
