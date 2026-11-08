package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/loader"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type PackImport struct {
	Path, Name, Description, Directory string
	IconURL                            string
	Optional                           bool
	SelectedOptional                   map[string]bool
	LocalFiles                         []string
	Source                             minecraft.PackSource
	Archive                            modpack.ArchiveIdentity
	Preview                            *PackPreview
}

var packBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

func (s *Service) ImportPack(ctx context.Context, request PackImport, progress func(minecraft.Progress), onLine func(string)) (minecraft.Instance, error) {
	if err := modpack.ValidateSource(request.Source); err != nil {
		return minecraft.Instance{}, err
	}
	name := strings.TrimSpace(request.Name)
	if name == "" {
		return minecraft.Instance{}, errors.New(locales.EnterInstanceName())
	}
	if request.Directory == "" {
		return minecraft.Instance{}, errors.New(locales.ChooseAnInstallLocation())
	}
	allowed := false
	for _, entry := range s.cfg.Directories.Entries {
		if entry.Path == request.Directory {
			allowed = true
			break
		}
	}
	if !allowed {
		return minecraft.Instance{}, errors.New(locales.InstallLocationHasNotBeenAdded())
	}
	if preview := request.Preview; preview != nil {
		v := preview.Version
		if request.Source.Provider != v.Source || request.Source.ProjectID != v.ProjectID || request.Source.VersionID != v.ID {
			return minecraft.Instance{}, errors.New(locales.TheModpackChangedSelectItAgain())
		}
		var err error
		request.Path, err = s.FetchPack(ctx, v, func(p download.Progress) {
			if progress != nil {
				progress(minecraft.Progress{Stage: "packfiles", Download: &p})
			}
		})
		if err != nil {
			return minecraft.Instance{}, err
		}
	}
	a, identity, err := s.openPack(ctx, request.Path)
	if err != nil {
		return minecraft.Instance{}, err
	}
	defer func() { _ = a.Close() }()
	if preview := request.Preview; preview != nil && !preview.Deferred && !reflect.DeepEqual(preview.Info, a.Info) {
		return minecraft.Instance{}, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	if request.Archive.SHA512 != "" && (request.Archive.Size != identity.Size || !strings.EqualFold(request.Archive.SHA512, identity.SHA512)) {
		return minecraft.Instance{}, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	a.Info = a.Info.WithOptional(request.SelectedOptional)
	origin, err := modpack.NewOrigin(a.Info, request.Source, identity, request.Optional)
	if err != nil {
		return minecraft.Instance{}, err
	}
	if len(a.Info.Warnings) > 0 {
		return minecraft.Instance{}, fmt.Errorf("%s", strings.Join(a.Info.Warnings, "；"))
	}
	prismParent, prism, err := minecraft.PrismInstallRoot(ctx, request.Directory)
	if err != nil {
		return minecraft.Instance{}, err
	}
	if prism && a.Profile != nil && len(a.Info.Loaders) == 0 {
		return minecraft.Instance{}, errors.New(locales.ThisModpackHasCustomVersionSettingsChooseARegularGameDirectoryInsteadOfPrism())
	}
	files, err := modpack.Resolve(ctx, s.client, a.Info, modpack.CurseForgeKey(s.cfg.Download.CurseForgeKey), request.Optional)
	if err != nil {
		return minecraft.Instance{}, err
	}
	resolved := a.Info
	resolved.Files = files
	needs, err := s.packSpaceRequirements(ctx, request.Directory, resolved, true)
	if err != nil {
		return minecraft.Instance{}, err
	}
	space, err := storage.Assess(ctx, needs)
	if progress != nil {
		progress(minecraft.Progress{Stage: "space", Space: &space})
	}
	if err != nil {
		return minecraft.Instance{}, err
	}
	if err := space.Check(); err != nil {
		return minecraft.Instance{}, err
	}
	sources, err := s.preparePackFiles(ctx, files, request.LocalFiles, progress)
	if err != nil {
		return minecraft.Instance{}, err
	}
	parent := filepath.Join(request.Directory, "instances")
	if prism {
		parent = prismParent
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return minecraft.Instance{}, err
	}
	stage, err := os.MkdirTemp(parent, ".import-*")
	if err != nil {
		return minecraft.Instance{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	root := filepath.Join(stage, "minecraft")
	if progress != nil {
		progress(minecraft.Progress{Stage: "prepare"})
	}
	var detail *minecraft.VersionDetail
	if prism {
		detail, err = s.installPrismStage(ctx, stage, name, a.Info.GameVersion, a.Info.Loaders, progress)
		if err != nil {
			return minecraft.Instance{}, err
		}
	} else {
		manifest, err := minecraft.FetchManifest(ctx, s.client)
		if err != nil {
			return minecraft.Instance{}, err
		}
		var summary minecraft.VersionSummary
		for _, v := range manifest.Versions {
			if v.ID == a.Info.GameVersion {
				summary = v
				break
			}
		}
		if summary.URL == "" {
			return minecraft.Instance{}, fmt.Errorf(locales.MinecraftNotFound(), a.Info.GameVersion)
		}
		base, err := minecraft.FetchVersion(ctx, s.client, summary)
		if err != nil {
			return minecraft.Instance{}, err
		}
		installer := s.installer(root, progress)
		installer.Isolate = false
		installer.ReuseRoots = s.installReuseRoots(request.Directory)
		installer.ClientSources = append([]string{installer.ClientJar(base.ClientID())}, installClientSources(base.ClientID(), installer.ReuseRoots)...)
		if err := installer.Install(ctx, base); err != nil {
			return minecraft.Instance{}, err
		}
		detail = base
		if len(a.Info.Loaders) > 0 {
			kind := a.Info.Loaders[0]
			var javaPath string
			if kind.ID == "forge" || kind.ID == "neoforge" {
				j, err := java.Ensure(ctx, s.client, base, s.discoverJava(ctx), s.cfg.Java.RuntimePath(base.RequiredJava()), s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
				if err != nil {
					return minecraft.Instance{}, err
				}
				javaPath = j.Path
			}
			if progress != nil {
				progress(minecraft.Progress{Stage: "loader"})
			}
			l := loader.Installer{Fetcher: s.client, Root: root, Cache: s.cacheDir, Java: javaPath, ReuseRoots: installer.ReuseRoots, Progress: progress, OnLine: onLine}
			detail, err = l.Install(ctx, base, kind)
			if err != nil {
				return minecraft.Instance{}, err
			}
		} else if a.Profile != nil {
			detail = minecraft.MergeVersion(base, a.Profile)
		}
		if detail != base {
			if err := installer.Install(ctx, detail); err != nil {
				return minecraft.Instance{}, err
			}
		}
	}
	for index, file := range files {
		dst, err := fsutil.SafePath(root, file.Path)
		if err != nil {
			return minecraft.Instance{}, err
		}
		if err := s.copyPackFileRecorded(ctx, sources[index], dst, file, origin.Put); err != nil {
			return minecraft.Instance{}, err
		}
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "extract"})
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return minecraft.Instance{}, err
	}
	err = a.ExtractRecorded(ctx, root, nil, origin.Put)
	release()
	if err != nil {
		return minecraft.Instance{}, err
	}
	stageRoot, err := os.OpenRoot(stage)
	if err != nil {
		return minecraft.Instance{}, err
	}
	err = origin.Write(ctx, stageRoot)
	_ = stageRoot.Close()
	if err != nil {
		return minecraft.Instance{}, err
	}
	if err := a.WriteIcon(ctx, stage); err != nil {
		return minecraft.Instance{}, err
	}
	if request.IconURL != "" {
		if _, err := os.Stat(filepath.Join(stage, "icon.png")); os.IsNotExist(err) {
			if icon, err := s.PackCover(ctx, request.IconURL); err == nil {
				var image bytes.Buffer
				if err := png.Encode(&image, icon); err == nil {
					_ = fsutil.WriteAtomic(filepath.Join(stage, "icon.png"), image.Bytes())
				}
			}
		}
	}
	if prism {
		if err := minecraft.WritePrismMetadata(stage, minecraft.PrismMetadata{Description: request.Description, PackFormat: a.Info.Format, PackVersion: a.Info.Version, PackSource: request.Source}); err != nil {
			return minecraft.Instance{}, err
		}
		return publishPrism(ctx, stage, parent)
	}
	var loaders []string
	for _, l := range a.Info.Loaders {
		loaders = append(loaders, l.ID)
	}
	meta := minecraft.ManagedMetadata{Format: 1, Name: name, Description: request.Description, Version: detail.ID, GameVersion: a.Info.GameVersion, Loader: a.Info.LoaderLabel(), Loaders: loaders, PackFormat: a.Info.Format, PackVersion: a.Info.Version, PackSource: request.Source, ImportedAt: time.Now()}
	if err := minecraft.WriteManaged(stage, meta); err != nil {
		return minecraft.Instance{}, err
	}
	if _, err := minecraft.ReadManaged(stage); err != nil {
		return minecraft.Instance{}, err
	}
	if err := ctx.Err(); err != nil {
		return minecraft.Instance{}, err
	}
	id := "pack-" + rand.Text()[:16]
	target := filepath.Join(parent, id)
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return minecraft.Instance{}, errors.New(locales.TargetInstanceAlreadyExists())
	}
	if err := os.Rename(stage, target); err != nil {
		return minecraft.Instance{}, err
	}
	instance, err := minecraft.ReadManaged(target)
	if err != nil {
		_ = os.RemoveAll(target)
		return minecraft.Instance{}, err
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "done"})
	}
	return instance, nil
}

func (s *Service) preparePackFiles(ctx context.Context, files []modpack.File, local []string, progress func(minecraft.Progress)) ([]string, error) {
	sources := make([]string, len(files))
	bySize := make(map[int64][]string)
	seenLocal := make(map[string]bool)
	var candidates []string
	for _, path := range local {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path = filepath.Clean(path)
		if seenLocal[path] {
			continue
		}
		seenLocal[path] = true
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
			bySize[info.Size()] = append(bySize[info.Size()], path)
			candidates = append(candidates, path)
		}
	}
	localSources := func(task download.Task) []string {
		if task.Size <= 0 {
			return candidates
		}
		return bySize[task.Size]
	}
	var tasks []download.Task
	var missing []modpack.MissingFile
	seen := make(map[string]bool)
	for index, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		key := strings.ToLower(filepath.Clean(f.Path))
		if seen[key] {
			return nil, fmt.Errorf(locales.DuplicateModpackFile(), f.Path)
		}
		seen[key] = true
		path := cache.FileIn(s.cacheDir, "downloads", strings.ToLower(f.SHA1))
		sources[index] = path
		if len(f.URLs) > 0 {
			tasks = append(tasks, download.Task{URL: f.URLs[0], Mirrors: f.URLs[1:], Path: path, SHA1: f.SHA1, SHA512: f.SHA512, Size: f.Size})
			continue
		}
		if s.packFileComplete(ctx, path, f) {
			continue
		}
		matched := false
		for _, p := range localSources(download.Task{Size: f.Size}) {
			if s.packFileComplete(ctx, p, f) {
				if err := s.copyPackFileRecorded(ctx, p, path, f, nil); err != nil {
					return nil, err
				}
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, modpack.MissingFile{Path: f.Path, Name: filepath.Base(f.Path), SHA1: f.SHA1, Size: f.Size, Website: f.Website})
		}
	}
	if len(missing) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, &modpack.MissingFilesError{Files: missing}
	}
	if err := s.client.FetchAllWithReuse(ctx, tasks, s.client.Workers, func(p download.Progress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "packfiles", Download: &p, Done: p.Done, Total: p.Total, Files: p.Files, FilesDone: p.FilesDone})
		}
	}, localSources); err != nil {
		return nil, err
	}
	return sources, nil
}

func (s *Service) packFileComplete(ctx context.Context, path string, file modpack.File) bool {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return false
	}
	defer release()
	return fsutil.CompleteHashesContext(ctx, path, file.SHA1, file.SHA512, file.Size)
}

func (s *Service) copyPackFileRecorded(ctx context.Context, src, dst string, file modpack.File, record func(modpack.InstalledFile) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := fsutil.OpenRegular(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if file.Size > 0 && before.Size() != file.Size {
		return fmt.Errorf(locales.VerificationFailed(), file.Path)
	}
	out, err := os.CreateTemp(filepath.Dir(dst), ".pack-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(out.Name()) }()
	h1, h512 := sha1.New(), sha512.New()
	writer := io.Writer(out)
	if file.SHA1 != "" || record != nil {
		writer = io.MultiWriter(writer, h1)
	}
	if file.SHA512 != "" {
		writer = io.MultiWriter(writer, h512)
	}
	buffer := packBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(writer, io.LimitReader(fsutil.ContextReader(ctx, in), before.Size()+1), *buffer)
	packBuffers.Put(buffer)
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	after, err := in.Stat()
	if err != nil {
		return err
	}
	if n != before.Size() || !sameExportFile(before, after) || file.SHA1 != "" && !strings.EqualFold(hex.EncodeToString(h1.Sum(nil)), file.SHA1) || file.SHA512 != "" && !strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), file.SHA512) {
		return fmt.Errorf(locales.VerificationFailed(), file.Path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(out.Name(), dst); err != nil {
		return err
	}
	if record != nil {
		return record(modpack.InstalledFile{Path: file.Path, SHA1: hex.EncodeToString(h1.Sum(nil)), Size: n, ProjectID: file.ProjectID, FileID: file.FileID, Optional: file.Optional})
	}
	return nil
}
