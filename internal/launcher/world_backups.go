package launcher

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldBackup struct {
	Name     string
	Size     int64
	Modified time.Time
	info     os.FileInfo
}

type WorldRestoreResult struct{ Path, Backup string }

func openWorldBackupRoot(content string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(content)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	for _, dir := range []string{"backups", filepath.Join("backups", "worlds")} {
		info, err := root.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) && create {
			err = root.Mkdir(dir, 0755)
		} else if err == nil && !info.IsDir() {
			err = errors.New(locales.InvalidBackupDirectory())
		}
		if err != nil {
			return nil, err
		}
	}
	return root.OpenRoot(filepath.Join("backups", "worlds"))
}

func worldBackupBytes(source WorldSource) uint64 {
	return storage.Add(storage.Add(source.Bytes, source.Bytes/16), storage.Add(uint64(source.Files)*512, 16<<20))
}

func (s *Service) WorldBackups(ctx context.Context, instance minecraft.Instance) ([]WorldBackup, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	root, err := openWorldBackupRoot(instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dir, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	sortBackups := func(items []WorldBackup) {
		slices.SortFunc(items, func(a, b WorldBackup) int {
			if n := b.Modified.Compare(a.Modified); n != 0 {
				return n
			}
			return strings.Compare(a.Name, b.Name)
		})
	}
	var result []WorldBackup
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := dir.ReadDir(64)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".zip") || strings.HasPrefix(entry.Name(), ".tomori-") {
				continue
			}
			info, e := entry.Info()
			if errors.Is(e, os.ErrNotExist) {
				continue
			}
			if e != nil {
				return nil, e
			}
			if info.Mode().IsRegular() {
				result = append(result, WorldBackup{Name: entry.Name(), Size: info.Size(), Modified: info.ModTime(), info: info})
			}
		}
		if len(result) > 512 {
			sortBackups(result)
			clear(result[256:])
			result = result[:256]
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	sortBackups(result)
	clear(result[min(256, len(result)):])
	return result[:min(256, len(result))], nil
}

func checkWorldBackup(root *os.Root, item WorldBackup) error {
	if err := fsutil.ValidateID(item.Name); err != nil {
		return err
	}
	if item.info == nil || !strings.EqualFold(filepath.Ext(item.Name), ".zip") {
		return errors.New(locales.BackupChangedRefreshAndTryAgain())
	}
	info, err := root.Lstat(item.Name)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !sameWorldSource(item.info, info) {
		return errors.New(locales.BackupChangedRefreshAndTryAgain())
	}
	return nil
}

func (s *Service) InspectWorldBackup(ctx context.Context, instance minecraft.Instance, item WorldBackup) (WorldSource, error) {
	root, err := openWorldBackupRoot(instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), false)
	if err != nil {
		return WorldSource{}, err
	}
	defer root.Close()
	if err := checkWorldBackup(root, item); err != nil {
		return WorldSource{}, err
	}
	source, err := s.InspectWorld(ctx, filepath.Join(root.Name(), item.Name))
	if err == nil && !sameWorldSource(item.info, source.info) {
		err = errors.New(locales.BackupChangedRefreshAndTryAgain())
	}
	return source, err
}

func (s *Service) TrashWorldBackup(ctx context.Context, instance minecraft.Instance, item WorldBackup, trash func(string) error) error {
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	roots, release, err := s.lockWorldRoots(ctx, nil, content)
	if err != nil {
		return err
	}
	defer release()
	permit, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer permit()
	root, err := openWorldBackupRoot(roots[0], false)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkWorldBackup(root, item); err != nil {
		return err
	}
	path := filepath.Join(root.Name(), item.Name)
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !sameWorldSource(item.info, info) {
		return errors.New(locales.BackupChangedRefreshAndTryAgain())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if trash == nil {
		return errors.New(locales.CouldNotMoveToTrash())
	}
	return trash(path)
}

func (s *Service) AssessWorldRestore(ctx context.Context, instance minecraft.Instance, source, target WorldSource) (storage.Report, error) {
	backup := worldBackupBytes(target)
	return storage.Assess(ctx, []storage.Requirement{{Directory: instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), Bytes: storage.Add(storage.Add(source.Bytes, 16<<20), storage.Add(backup, backup))}})
}

func (s *Service) RestoreWorld(ctx context.Context, instance minecraft.Instance, source, target WorldSource, progress func(minecraft.Progress)) (result WorldRestoreResult, err error) {
	if !instance.CanLaunch() || source.Directory || source.info == nil || !target.Directory || target.info == nil {
		return result, errors.New(locales.InvalidWorldDirectory())
	}
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	roots, release, err := s.lockWorldRoots(ctx, progress, content)
	if err != nil {
		return result, err
	}
	defer release()
	content = roots[0]
	if filepath.Clean(target.Path) != filepath.Join(content, "saves", filepath.Base(target.Path)) {
		return result, errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	w, err := openWorldDirectory(content, filepath.Base(target.Path))
	if err != nil {
		return result, err
	}
	defer w.Close()
	if !os.SameFile(target.info, w.info) {
		return result, errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	if err := s.verifyWorldLevel(ctx, w.root, target.levelHash); err != nil {
		return result, err
	}
	current, err := s.InspectWorld(ctx, w.path)
	if err != nil {
		return result, err
	}
	space, err := s.AssessWorldRestore(ctx, instance, source, current)
	if err != nil {
		return result, err
	}
	if err := space.Check(); err != nil {
		return result, err
	}
	name := target.World.Name
	if name == "" {
		name = filepath.Base(target.Path)
	}
	result.Path, err = s.importWorldZip(ctx, instance, source, content, name, progress, func(parent *os.Root, directory, stage string) (string, error) {
		backup, err := s.backupWorld(ctx, w, content, current, progress)
		if err != nil {
			return "", err
		}
		result.Backup = backup
		if err := s.verifyWorldLevel(ctx, w.root, current.levelHash); err != nil {
			return "", err
		}
		return s.replaceWorld(ctx, w, parent, directory, stage)
	})
	return result, err
}

func (s *Service) replaceWorld(ctx context.Context, w *worldDirectory, parent *os.Root, directory, stage string) (string, error) {
	if err := w.Check(); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	folder := filepath.Base(w.path)
	current, err := parent.Lstat(folder)
	if err != nil {
		return "", err
	}
	if !os.SameFile(w.info, current) {
		return "", errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	previous := ".tomori-world-previous-" + rand.Text()
	if err := parent.Rename(folder, previous); err != nil {
		return "", err
	}
	rollback := func(cause error) (string, error) {
		if _, err := parent.Lstat(folder); errors.Is(err, os.ErrNotExist) {
			return "", errors.Join(cause, parent.Rename(previous, folder))
		}
		return "", errors.Join(cause, fmt.Errorf(locales.TheOriginalWorldIsPreservedAt(), filepath.Join(directory, previous)))
	}
	if err := ctx.Err(); err != nil {
		return rollback(err)
	}
	if err := parent.Rename(filepath.Base(stage), folder); err != nil {
		return rollback(err)
	}
	permit, err := s.client.FilePermit(context.WithoutCancel(ctx))
	if err == nil {
		current, err = parent.Lstat(previous)
		if err == nil && !os.SameFile(w.info, current) {
			err = errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
		}
		if err == nil {
			err = parent.RemoveAll(previous)
		}
		permit()
	}
	if err != nil {
		return filepath.Join(directory, folder), fmt.Errorf(locales.RestoredButTheOldFolderCouldNotBeRemoved(), err)
	}
	return filepath.Join(directory, folder), nil
}
