package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) openWorldResources(ctx context.Context, instance minecraft.Instance, folder string, progress func(minecraft.Progress)) (*worldDirectory, func(), error) {
	if !instance.CanLaunch() {
		return nil, nil, errors.New(locales.InvalidTargetInstance())
	}
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	roots, release, err := s.lockWorldRoots(ctx, progress, content)
	if err != nil {
		return nil, nil, err
	}
	w, err := openWorldDirectory(roots[0], folder)
	if err != nil {
		release()
		return nil, nil, err
	}
	info, err := w.root.Lstat("datapacks")
	if errors.Is(err, os.ErrNotExist) {
		err = w.root.Mkdir("datapacks", 0755)
	} else if err == nil && !info.IsDir() {
		err = errors.New(locales.InvalidResourceDirectory())
	}
	if err != nil {
		_ = w.Close()
		release()
		return nil, nil, err
	}
	return w, func() { _ = w.Close(); release() }, nil
}

func (s *Service) ScanWorldPacks(ctx context.Context, instance minecraft.Instance, folder string) ([]resource.Entry, error) {
	w, closeWorld, err := s.openWorldResources(ctx, instance, folder, nil)
	if err != nil {
		return nil, err
	}
	defer closeWorld()
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	packs, err := instancecontent.ReadWorldPacks(ctx, w.root)
	if err != nil {
		return nil, err
	}
	entries, err := resource.ScanWithOrigins(ctx, s.store.Dir(), w.path, resource.DataPack)
	for i := range entries {
		entries[i].Enabled = packs.IsEnabled(entries[i].File)
	}
	return entries, err
}

func (s *Service) ChangeWorldPacks(ctx context.Context, instance minecraft.Instance, folder, action string, entries []resource.Entry) (int, error) {
	if action != "enable" && action != "disable" && action != "delete" || len(entries) == 0 || len(entries) > 1000 {
		return 0, errors.New(locales.InvalidWorldAction())
	}
	w, closeWorld, err := s.openWorldResources(ctx, instance, folder, nil)
	if err != nil {
		return 0, err
	}
	defer closeWorld()
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	root, err := w.root.OpenRoot("datapacks")
	if err != nil {
		return 0, err
	}
	defer root.Close()
	changes := make(map[string]int, len(entries))
	for _, entry := range entries {
		if err := fsutil.ValidateID(entry.File); err != nil {
			return 0, err
		}
		if entry.File != entry.Relative || strings.HasPrefix(entry.File, ".") || !resource.DataPack.Accepts(entry.File, entry.Directory) {
			return 0, errors.New(locales.InvalidResourcePath())
		}
		info, err := root.Lstat(entry.File)
		if err != nil {
			return 0, err
		}
		if info.IsDir() != entry.Directory || !info.IsDir() && !info.Mode().IsRegular() || info.Size() != entry.Size || !info.ModTime().Equal(entry.Modified) {
			return 0, errors.New(locales.ResourceChangedRefreshTheList())
		}
		value := 0
		switch action {
		case "enable":
			value = 1
		case "disable":
			value = -1
		}
		changes[entry.File] = value
	}
	if err := w.Check(); err != nil {
		return 0, err
	}
	if action != "delete" {
		err := instancecontent.ChangeWorldPacks(ctx, w.root, changes)
		if err != nil {
			return 0, err
		}
		return len(changes), nil
	}
	stage, err := newCopyDirectory(root, filepath.Join(w.path, "datapacks"), ".tomori-remove-")
	if err != nil {
		return 0, err
	}
	stage = filepath.Base(stage)
	var moved []string
	rollback := func(cause error) error {
		for i := len(moved) - 1; i >= 0; i-- {
			name := moved[i]
			if _, err := root.Lstat(name); !errors.Is(err, os.ErrNotExist) {
				return errors.Join(cause, fmt.Errorf(locales.CouldNotRestoreResourceBackupAt(), filepath.Join(w.path, "datapacks", stage), errors.New(locales.ResourceChangedRefreshTheList())))
			}
			if err := root.Rename(filepath.Join(stage, name), name); err != nil {
				return errors.Join(cause, err)
			}
		}
		return errors.Join(cause, root.Remove(stage))
	}
	for _, entry := range entries {
		if _, exists := changes[entry.File]; !exists {
			continue
		}
		delete(changes, entry.File)
		if err := ctx.Err(); err != nil {
			return 0, rollback(err)
		}
		if err := root.Rename(entry.File, filepath.Join(stage, entry.File)); err != nil {
			return 0, rollback(err)
		}
		moved = append(moved, entry.File)
	}
	for _, file := range moved {
		changes[file] = 0
	}
	if err := instancecontent.ChangeWorldPacks(ctx, w.root, changes); err != nil {
		return 0, rollback(err)
	}
	return len(moved), root.RemoveAll(stage)
}

func (s *Service) ImportWorldPacks(ctx context.Context, instance minecraft.Instance, folder string, paths []string) (int, error) {
	if len(paths) == 0 || len(paths) > 1000 {
		return 0, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	w, closeWorld, err := s.openWorldResources(ctx, instance, folder, nil)
	if err != nil {
		return 0, err
	}
	defer closeWorld()
	type importedPack struct {
		name string
		info os.FileInfo
	}
	var imported []importedPack
	rollback := func(cause error) error {
		for i := len(imported) - 1; i >= 0; i-- {
			item := imported[i]
			path := filepath.Join("datapacks", item.name)
			info, err := w.root.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err == nil && (!os.SameFile(item.info, info) || !info.IsDir() && (item.info.Size() != info.Size() || !item.info.ModTime().Equal(info.ModTime()))) {
				err = errors.New(locales.ResourceChangedRefreshTheList())
			}
			if err == nil {
				err = w.root.RemoveAll(path)
			}
			cause = errors.Join(cause, err)
		}
		return cause
	}
	record := func(name string, info os.FileInfo) { imported = append(imported, importedPack{name, info}) }
	var problems []error
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return 0, rollback(err)
		}
		if err := s.importWorldPack(ctx, w, path, record); err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", filepath.Base(path), err))
		}
	}
	if len(imported) == 0 {
		return 0, errors.Join(problems...)
	}
	if err := w.Check(); err != nil {
		return 0, rollback(errors.Join(err, errors.Join(problems...)))
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return 0, rollback(errors.Join(err, errors.Join(problems...)))
	}
	changes := make(map[string]int, len(imported))
	for _, item := range imported {
		changes[item.name] = 1
	}
	err = instancecontent.ChangeWorldPacks(ctx, w.root, changes)
	release()
	if err != nil {
		return 0, rollback(errors.Join(err, errors.Join(problems...)))
	}
	return len(imported), errors.Join(problems...)
}

func (s *Service) importWorldPack(ctx context.Context, w *worldDirectory, path string, record func(string, os.FileInfo)) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return s.importWorldPackFolder(ctx, w, path, info, record)
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	if !info.Mode().IsRegular() || !resource.DataPack.Accepts(info.Name(), false) {
		return errors.New(locales.ChooseADataPackZipFile())
	}
	if info.Size() > 4<<30 {
		return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
	}
	if _, err := resource.Inspect(ctx, path, resource.DataPack); err != nil {
		return err
	}
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: w.path, Bytes: storage.Add(uint64(max(0, info.Size())), 16<<20)}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	hash, err := resource.Hash(ctx, path)
	if err != nil {
		return err
	}
	imported, err := resource.ImportVerified(ctx, w.path, resource.DataPack, path, hash)
	if err != nil {
		return err
	}
	defer imported.Close()
	if err := w.Check(); err != nil {
		return errors.Join(err, imported.Undo())
	}
	record(info.Name(), imported.FileInfo())
	return nil
}

func (s *Service) importWorldPackFolder(ctx context.Context, w *worldDirectory, path string, info os.FileInfo, record func(string, os.FileInfo)) (err error) {
	name := info.Name()
	if err := fsutil.ValidateID(name); err != nil {
		return err
	}
	if !resource.DataPack.Accepts(name, true) {
		return errors.New(locales.InvalidResourcePath())
	}
	source, err := os.OpenRoot(path)
	if err != nil {
		return err
	}
	defer source.Close()
	opened, err := source.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return errors.New(locales.ResourceChangedRefreshTheList())
	}
	parentPath := filepath.Join(w.path, "datapacks")
	realSource, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if pathWithin(realSource, parentPath) {
		return errors.New(locales.CopyDestinationCannotBeInsideTheSourceWorld())
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	var size uint64
	_, err = resource.Inspect(ctx, path, resource.DataPack)
	if err == nil {
		err = fsutil.WalkFiles(ctx, source, nil, func(_ string, file os.FileInfo) error {
			if file.Size() < 0 || uint64(file.Size()) > 4<<30-size {
				return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
			}
			size += uint64(file.Size())
			return nil
		})
	}
	release()
	if err != nil {
		return err
	}
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: w.path, Bytes: storage.Add(size, 16<<20)}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	parent, err := w.root.OpenRoot("datapacks")
	if err != nil {
		return err
	}
	defer parent.Close()
	for _, existing := range []string{name, name + ".disabled"} {
		if _, err := parent.Lstat(existing); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			return errors.New(locales.AResourceWithThisNameAlreadyExists())
		}
	}
	stage, err := newCopyDirectory(parent, parentPath, ".tomori-datapack-")
	if err != nil {
		return err
	}
	stage = filepath.Base(stage)
	stageInfo, err := parent.Lstat(stage)
	if err != nil {
		return err
	}
	defer func() {
		if current, e := parent.Lstat(stage); e == nil && os.SameFile(stageInfo, current) {
			err = errors.Join(err, parent.RemoveAll(stage))
		}
	}()
	target, err := parent.OpenRoot(stage)
	if err != nil {
		return err
	}
	copier := fsutil.NewTreeCopier(ctx, target, s.client.FilePermit, nil)
	err = copier.Tree(source, nil)
	if err != nil {
		copier.Cancel()
	}
	if e := copier.Close(); err == nil {
		err = e
	}
	if e := target.Close(); err == nil {
		err = e
	}
	if err != nil {
		return err
	}
	if err := w.Check(); err != nil {
		return err
	}
	if _, err := parent.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return errors.New(locales.AResourceWithThisNameAlreadyExists())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := parent.Rename(stage, name); err != nil {
		return err
	}
	record(name, stageInfo)
	return nil
}
