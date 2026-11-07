package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldChange struct {
	Action, Folder, Name string
	Target               minecraft.Instance
}

type worldDirectory struct {
	root, parent *os.Root
	lock         *os.File
	path         string
	info         os.FileInfo
}

func openWorldDirectory(content, folder string) (_ *worldDirectory, err error) {
	return openWorldPath(filepath.Join(content, "saves"), folder)
}

func openWorldPath(directory, folder string) (_ *worldDirectory, err error) {
	if folder == "" || folder == "." || folder == ".." || strings.ContainsAny(folder, "/\\\x00") {
		return nil, errors.New(locales.InvalidWorldDirectory())
	}
	parent, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	w := &worldDirectory{parent: parent, path: filepath.Join(directory, folder)}
	defer func() {
		if err != nil {
			_ = w.Close()
		}
	}()
	w.info, err = parent.Lstat(folder)
	if err != nil {
		return nil, err
	}
	if !w.info.IsDir() {
		return nil, errors.New(locales.InvalidWorldDirectory())
	}
	w.root, err = parent.OpenRoot(folder)
	if err != nil {
		return nil, err
	}
	opened, err := w.root.Stat(".")
	if err != nil {
		return nil, err
	}
	if !os.SameFile(w.info, opened) {
		return nil, errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	level, err := fsutil.OpenRootRegular(w.root, "level.dat")
	if err != nil {
		return nil, err
	}
	if err := level.Close(); err != nil {
		return nil, err
	}
	info, statErr := w.root.Lstat("session.lock")
	flags := os.O_RDWR | worldLockFlags
	if errors.Is(statErr, os.ErrNotExist) {
		flags |= os.O_CREATE | os.O_EXCL
	} else if statErr != nil {
		return nil, statErr
	} else if !info.Mode().IsRegular() {
		return nil, errors.New(locales.InvalidWorldDirectory())
	}
	w.lock, err = w.root.OpenFile("session.lock", flags, 0600)
	if err != nil {
		return nil, err
	}
	locked, err := w.lock.Stat()
	if err != nil {
		return nil, err
	}
	if !locked.Mode().IsRegular() || info != nil && !os.SameFile(info, locked) {
		return nil, errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	if err := lockWorldFile(w.lock); err != nil {
		return nil, fmt.Errorf(locales.WorldIsInUseOrCouldNotBeLocked(), err)
	}
	if err := w.Check(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *worldDirectory) Check() error {
	info, err := os.Lstat(w.path)
	if err != nil {
		return err
	}
	if !os.SameFile(w.info, info) {
		return errors.New(locales.WorldDirectoryChangedRefreshAndTryAgain())
	}
	return nil
}

func (w *worldDirectory) Close() error {
	var err error
	if w.lock != nil {
		err = w.lock.Close()
		w.lock = nil
	}
	if w.root != nil {
		err = errors.Join(err, w.root.Close())
		w.root = nil
	}
	if w.parent != nil {
		err = errors.Join(err, w.parent.Close())
		w.parent = nil
	}
	return err
}

func (s *Service) ChangeWorld(ctx context.Context, instance minecraft.Instance, change WorldChange, progress func(minecraft.Progress), trash func(string) error) (string, error) {
	if change.Action != "rename" && change.Action != "copy" && change.Action != "delete" {
		return "", errors.New(locales.InvalidWorldAction())
	}
	if change.Action != "delete" {
		name, err := instancecontent.WorldName(change.Name)
		if err != nil {
			return "", err
		}
		change.Name = name
	}
	source := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	target := source
	if change.Action == "copy" {
		if !change.Target.CanLaunch() {
			return "", errors.New(locales.InvalidTargetInstance())
		}
		target = change.Target.ContentDirectory(s.cfg.GameForInstance(change.Target.Key()).Isolate)
	}
	if source == "" || target == "" {
		return "", errors.New(locales.InvalidWorldDirectory())
	}
	roots, release, err := s.lockWorldRoots(ctx, progress, source, target)
	if err != nil {
		return "", err
	}
	defer release()
	source, target = roots[0], roots[1]
	w, err := openWorldDirectory(source, change.Folder)
	if err != nil {
		return "", err
	}
	defer w.Close()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	switch change.Action {
	case "rename":
		if err := s.renameWorld(ctx, w.root, change.Name); err != nil {
			return "", err
		}
		return w.path, w.Check()
	case "delete":
		if trash == nil {
			return "", errors.New(locales.CouldNotMoveToTrash())
		}
		if err := w.Check(); err != nil {
			return "", err
		}
		if err := w.Close(); err != nil {
			return "", err
		}
		if err := w.Check(); err != nil {
			return "", err
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return "", trash(w.path)
	default:
		return s.copyWorld(ctx, w, target, change.Name, progress)
	}
}

func (s *Service) lockWorldRoots(ctx context.Context, progress func(minecraft.Progress), roots ...string) ([]string, func(), error) {
	for i, root := range roots {
		real, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, nil, err
		}
		real, err = filepath.Abs(real)
		if err != nil {
			return nil, nil, err
		}
		if runtime.GOOS == "windows" {
			real = strings.ToLower(real)
		}
		roots[i] = real
	}
	keys := slices.Clone(roots)
	slices.Sort(keys)
	keys = slices.Compact(keys)
	var releases []func()
	release := func() {
		for _, release := range slices.Backward(releases) {
			release()
		}
	}
	for _, root := range keys {
		unlock, err := s.lockRoot(ctx, root, progress)
		if err != nil {
			release()
			return nil, nil, err
		}
		releases = append(releases, unlock)
	}
	return roots, release, nil
}

func worldCopyContent(path string, _ bool) bool {
	base := filepath.Base(path)
	return base != "session.lock" && !strings.HasPrefix(base, ".tomori-write-")
}

func (s *Service) renameWorld(ctx context.Context, root *os.Root, name string) error {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	return instancecontent.RenameWorld(ctx, root, name)
}

func (s *Service) copyWorld(ctx context.Context, source *worldDirectory, content, name string, progress func(minecraft.Progress)) (_ string, err error) {
	if pathWithin(source.path, content) {
		return "", errors.New(locales.CopyDestinationCannotBeInsideTheSourceWorld())
	}
	var size uint64
	err = fsutil.WalkFiles(ctx, source.root, worldCopyContent, func(_ string, info os.FileInfo) error {
		if info.Size() < 0 || uint64(info.Size()) > 64<<30-size {
			return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
		}
		size += uint64(info.Size())
		return nil
	})
	if err != nil {
		return "", err
	}
	report, err := storage.Assess(ctx, []storage.Requirement{{Directory: content, Bytes: storage.Add(size, 16<<20)}})
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
	realParent, err := filepath.EvalSymlinks(parentPath)
	if err != nil {
		return "", err
	}
	if pathWithin(source.path, realParent) {
		return "", errors.New(locales.CopyDestinationCannotBeInsideTheSourceWorld())
	}
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
		if current, statErr := parent.Lstat(stageName); statErr == nil && os.SameFile(stageInfo, current) {
			err = errors.Join(err, parent.RemoveAll(stageName))
		}
	}()
	destination, err := parent.OpenRoot(stageName)
	if err != nil {
		return "", err
	}
	copier := fsutil.NewTreeCopier(ctx, destination, s.client.FilePermit, func(p fsutil.CopyProgress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "copy", Done: p.Bytes, Total: int64(size), Files: p.TotalFiles, FilesDone: p.Files})
		}
	})
	err = copier.Tree(source.root, worldCopyContent)
	if err != nil {
		copier.Cancel()
	}
	if closeErr := copier.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = s.renameWorld(ctx, destination, name)
	}
	if closeErr := destination.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err := source.Check(); err != nil {
		return "", err
	}
	result, _, err := publishCopy(ctx, parent, parentPath, stage, "level.dat", "world-")
	return result, err
}
