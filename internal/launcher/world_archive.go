package launcher

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldSource struct {
	Path      string
	Directory bool
	World     instancecontent.World
	Bytes     uint64
	Files     int
	info      os.FileInfo
	levelHash [32]byte
}

func (s *Service) InspectWorld(ctx context.Context, name string) (WorldSource, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return WorldSource{}, err
	}
	defer release()
	return inspectWorldSource(ctx, name)
}

func inspectWorldSource(ctx context.Context, name string) (result WorldSource, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result.Path, err = filepath.Abs(name)
	if err == nil {
		result.Path, err = filepath.EvalSymlinks(result.Path)
	}
	if err == nil {
		result.info, err = os.Lstat(result.Path)
	}
	if err != nil {
		return result, err
	}
	result.Directory = result.info.IsDir()
	var raw []byte
	if result.Directory {
		root, err := os.OpenRoot(result.Path)
		if err != nil {
			return result, err
		}
		defer root.Close()
		raw, err = fsutil.ReadRootFile(root, "level.dat", 8<<20)
		if err != nil {
			return result, err
		}
		err = fsutil.WalkFiles(ctx, root, worldCopyContent, func(_ string, info os.FileInfo) error {
			if info.Size() < 0 || info.Size() > 4<<30 || uint64(info.Size()) > 64<<30-result.Bytes {
				return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
			}
			result.Bytes += uint64(info.Size())
			result.Files++
			return nil
		})
		if err != nil {
			return result, err
		}
	} else {
		f, reader, err := openWorldZip(ctx, result.Path)
		if err != nil {
			return result, err
		}
		defer f.Close()
		prefix, level, err := worldZipLayout(ctx, reader)
		if err != nil {
			return result, err
		}
		result.Bytes, result.Files, err = worldZipSize(ctx, reader, prefix)
		if err != nil {
			return result, err
		}
		raw, err = fsutil.ReadZipFile(ctx, level, 8<<20)
		if err != nil {
			return result, err
		}
	}
	result.World, err = instancecontent.ReadWorldData(ctx, raw)
	result.levelHash = sha256.Sum256(raw)
	if err == nil {
		current, e := os.Lstat(result.Path)
		if e != nil {
			err = e
		} else if !sameWorldSource(result.info, current) {
			err = errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
	}
	return result, err
}

func sameWorldSource(a, b os.FileInfo) bool {
	return os.SameFile(a, b) && a.Mode().Type() == b.Mode().Type() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func openWorldZip(ctx context.Context, name string) (*os.File, *zip.Reader, error) {
	f, err := fsutil.OpenRegular(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	var reader *zip.Reader
	if err == nil {
		reader, err = fsutil.NewZipReader(ctx, f, info.Size())
	}
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, reader, nil
}

func worldZipName(file *zip.File) (string, error) {
	name := strings.ReplaceAll(file.Name, "\\", "/")
	if file.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), file.Name)
	}
	if file.FileInfo().IsDir() {
		name = strings.TrimSuffix(name, "/")
	} else if !file.Mode().IsRegular() {
		return "", fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), file.Name)
	}
	if name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || len(name) > 4096 || strings.Count(name, "/") > 128 {
		return "", errors.New(locales.InvalidWorldArchivePath())
	}
	for part := range strings.SplitSeq(name, "/") {
		if err := fsutil.ValidateID(part); err != nil {
			return "", err
		}
	}
	return name, nil
}

func worldZipLayout(ctx context.Context, reader *zip.Reader) (string, *zip.File, error) {
	seen := make(map[string]bool)
	pathBytes, depth, candidates := 0, 129, 0
	var level *zip.File
	prefix := ""
	for _, file := range reader.File {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		name, err := worldZipName(file)
		if err != nil {
			return "", nil, err
		}
		key := strings.ToLower(name)
		if seen[key] {
			return "", nil, fmt.Errorf(locales.DuplicateArchivePath(), file.Name)
		}
		seen[key] = true
		pathBytes += len(name)
		if pathBytes > 16<<20 {
			return "", nil, errors.New(locales.ArchiveDirectorySizeOrEntryCountExceedsTheLimit())
		}
		if path.Base(name) != "level.dat" || !file.Mode().IsRegular() {
			continue
		}
		d := strings.Count(name, "/")
		if d < depth {
			depth, candidates, level = d, 1, file
			prefix = strings.TrimSuffix(name, "level.dat")
		} else if d == depth {
			candidates++
		}
	}
	if candidates != 1 {
		return "", nil, errors.New(locales.ChooseAnArchiveContainingOneWorld())
	}
	return prefix, level, nil
}

func worldZipSize(ctx context.Context, reader *zip.Reader, prefix string) (size uint64, files int, err error) {
	for _, f := range reader.File {
		if err := ctx.Err(); err != nil {
			return 0, 0, err
		}
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if f.FileInfo().IsDir() || !strings.HasPrefix(name, prefix) || !worldCopyContent(filepath.FromSlash(strings.TrimPrefix(name, prefix)), false) {
			continue
		}
		if f.UncompressedSize64 > 4<<30 || f.UncompressedSize64 > 64<<30-size {
			return 0, 0, errors.New(locales.ArchiveExtractionSizeOrFileCountExceedsLimit())
		}
		size += f.UncompressedSize64
		files++
	}
	return size, files, nil
}

func (s *Service) AssessWorldImport(ctx context.Context, instance minecraft.Instance, source WorldSource) (storage.Report, error) {
	return storage.Assess(ctx, []storage.Requirement{{Directory: instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), Bytes: storage.Add(source.Bytes, 16<<20)}})
}

func (s *Service) AssessWorldBackup(ctx context.Context, instance minecraft.Instance, source WorldSource) (storage.Report, error) {
	required := worldBackupBytes(source)
	return storage.Assess(ctx, []storage.Requirement{{Directory: instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), Bytes: storage.Add(required, required)}})
}

func (s *Service) ImportWorld(ctx context.Context, instance minecraft.Instance, preview WorldSource, name string, progress func(minecraft.Progress)) (_ string, err error) {
	if !instance.CanLaunch() || preview.info == nil {
		return "", errors.New(locales.InvalidWorldDirectory())
	}
	if _, err := instancecontent.WorldName(name); err != nil {
		return "", err
	}
	source := preview
	current, err := os.Lstat(source.Path)
	if err != nil {
		return "", err
	}
	if !sameWorldSource(preview.info, current) {
		return "", errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	lockPaths := []string{content}
	if source.Directory {
		sourceLock := source.Path
		if filepath.Base(filepath.Dir(source.Path)) == "saves" {
			sourceLock = filepath.Dir(filepath.Dir(source.Path))
		}
		lockPaths = append(lockPaths, sourceLock)
	}
	roots, release, err := s.lockWorldRoots(ctx, progress, lockPaths...)
	if err != nil {
		return "", err
	}
	defer release()
	content = roots[0]
	if source.Directory {
		w, err := openWorldPath(filepath.Dir(source.Path), filepath.Base(source.Path))
		if err != nil {
			return "", err
		}
		defer w.Close()
		if !os.SameFile(preview.info, w.info) {
			return "", errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
		}
		if err := s.verifyWorldLevel(ctx, w.root, preview.levelHash); err != nil {
			return "", err
		}
		return s.copyWorld(ctx, w, content, name, progress)
	}
	return s.importWorldZip(ctx, instance, source, content, name, progress, nil)
}

func (s *Service) importWorldZip(ctx context.Context, instance minecraft.Instance, source WorldSource, content, name string, progress func(minecraft.Progress), publish func(*os.Root, string, string) (string, error)) (_ string, err error) {
	f, reader, prefix, err := s.prepareWorldZip(ctx, &source)
	if err != nil {
		return "", err
	}
	defer f.Close()
	report, err := s.AssessWorldImport(ctx, instance, source)
	if err != nil {
		return "", err
	}
	if err := report.Check(); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(content)
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err := root.MkdirAll("saves", 0755); err != nil {
		return "", err
	}
	parent, err := root.OpenRoot("saves")
	if err != nil {
		return "", err
	}
	defer parent.Close()
	parentPath := filepath.Join(content, "saves")
	stage, err := newCopyDirectory(parent, parentPath, ".tomori-world-")
	if err != nil {
		return "", err
	}
	stageName := filepath.Base(stage)
	stageInfo, err := parent.Lstat(stageName)
	if err != nil {
		return "", err
	}
	defer func() {
		if current, e := parent.Lstat(stageName); e == nil && os.SameFile(stageInfo, current) {
			err = errors.Join(err, parent.RemoveAll(stageName))
		}
	}()
	destination, err := parent.OpenRoot(stageName)
	if err != nil {
		return "", err
	}
	writer, err := fsutil.NewRootArchiveWriter(destination, 200000, 4<<30, 64<<30)
	if err != nil {
		_ = destination.Close()
		return "", err
	}
	var done uint64
	files, last := 0, time.Time{}
	for _, entry := range reader.File {
		entryName := strings.ReplaceAll(entry.Name, "\\", "/")
		if entry.FileInfo().IsDir() || !strings.HasPrefix(entryName, prefix) {
			continue
		}
		relative := strings.TrimPrefix(entryName, prefix)
		if !worldCopyContent(filepath.FromSlash(relative), false) {
			continue
		}
		permit, e := s.client.FilePermit(ctx)
		if e != nil {
			err = e
			break
		}
		err = writer.WriteZip(ctx, entry, relative)
		permit()
		if err != nil {
			break
		}
		done += entry.UncompressedSize64
		files++
		if progress != nil && (time.Since(last) >= 100*time.Millisecond || files == source.Files) {
			last = time.Now()
			progress(minecraft.Progress{Stage: "world-import", Done: int64(done), Total: int64(source.Bytes), Files: source.Files, FilesDone: files})
		}
	}
	if e := writer.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = s.renameWorld(ctx, destination, name)
	}
	if e := destination.Close(); err == nil {
		err = e
	}
	if err != nil {
		return "", err
	}
	after, err := os.Lstat(source.Path)
	if err != nil || !sameWorldSource(source.info, after) {
		return "", errors.Join(err, errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain()))
	}
	if publish != nil {
		return publish(parent, parentPath, stage)
	}
	result, _, err := publishCopy(ctx, parent, parentPath, stage, "level.dat", "world-")
	return result, err
}

func (s *Service) verifyWorldLevel(ctx context.Context, root *os.Root, expected [32]byte) error {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	raw, err := fsutil.ReadRootFile(root, "level.dat", 8<<20)
	if err != nil {
		return err
	}
	if sha256.Sum256(raw) != expected {
		return errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	return ctx.Err()
}

func (s *Service) prepareWorldZip(ctx context.Context, source *WorldSource) (f *os.File, reader *zip.Reader, prefix string, err error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	defer release()
	f, reader, err = openWorldZip(ctx, source.Path)
	if err != nil {
		return nil, nil, "", err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
		}
	}()
	opened, err := f.Stat()
	if err != nil || !sameWorldSource(source.info, opened) {
		return f, nil, "", errors.Join(err, errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain()))
	}
	prefix, level, err := worldZipLayout(ctx, reader)
	if err != nil {
		return f, nil, "", err
	}
	source.Bytes, source.Files, err = worldZipSize(ctx, reader, prefix)
	if err != nil {
		return f, nil, "", err
	}
	raw, err := fsutil.ReadZipFile(ctx, level, 8<<20)
	if err != nil {
		return f, nil, "", err
	}
	if sha256.Sum256(raw) != source.levelHash {
		return f, nil, "", errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	return f, reader, prefix, nil
}

func (s *Service) BackupWorld(ctx context.Context, instance minecraft.Instance, folder string, progress func(minecraft.Progress)) (_ string, err error) {
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	roots, release, err := s.lockWorldRoots(ctx, progress, content)
	if err != nil {
		return "", err
	}
	defer release()
	content = roots[0]
	w, err := openWorldDirectory(content, folder)
	if err != nil {
		return "", err
	}
	defer w.Close()
	source, err := s.InspectWorld(ctx, w.path)
	if err != nil {
		return "", err
	}
	return s.backupWorld(ctx, w, content, source, progress)
}

func (s *Service) backupWorld(ctx context.Context, w *worldDirectory, content string, source WorldSource, progress func(minecraft.Progress)) (_ string, err error) {
	required := worldBackupBytes(source)
	report, err := storage.Assess(ctx, []storage.Requirement{{Directory: content, Bytes: storage.Add(required, required)}})
	if err != nil {
		return "", err
	}
	if err := report.Check(); err != nil {
		return "", err
	}
	destination, err := openWorldBackupRoot(content, true)
	if err != nil {
		return "", err
	}
	defer destination.Close()
	real, err := filepath.EvalSymlinks(destination.Name())
	if err != nil {
		return "", err
	}
	if pathWithin(source.Path, real) {
		return "", errors.New(locales.TheBackupDirectoryCannotBeInsideTheSourceWorld())
	}
	temp := ".tomori-world-" + rand.Text()
	out, err := destination.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	tempInfo, err := out.Stat()
	if err != nil {
		_ = out.Close()
		return "", err
	}
	defer func() {
		_ = out.Close()
		if current, e := destination.Lstat(temp); e == nil && os.SameFile(tempInfo, current) {
			_ = destination.Remove(temp)
		}
	}()
	z := fsutil.NewZipExporter(ctx, out, s.client.FilePermit, func(p fsutil.CopyProgress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "world-backup", Done: p.Bytes, Total: int64(source.Bytes), Files: source.Files, FilesDone: p.Files})
		}
	})
	err = z.Tree(w.root, "", worldCopyContent, nil)
	if e := z.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = w.Check()
	}
	if err == nil {
		err = out.Sync()
	}
	info, statErr := out.Stat()
	if err == nil {
		err = statErr
	}
	if e := out.Close(); err == nil {
		err = e
	}
	if err != nil {
		return "", err
	}
	filename := ExportFilename(source.World.Name) + "-" + time.Now().Format("20060102-150405") + "-" + rand.Text()[:8] + ".zip"
	releaseFile, err := s.client.FilePermit(ctx)
	if err != nil {
		return "", err
	}
	err = fsutil.PublishRootExclusive(ctx, destination, temp, filename, info)
	releaseFile()
	if err != nil {
		return "", err
	}
	return filepath.Join(destination.Name(), filename), nil
}
