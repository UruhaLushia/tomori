package resource

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func Scan(ctx context.Context, gameDir string, kind Kind) ([]Entry, error) {
	dir, err := Directory(gameDir, kind)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	var list []Entry
	subdirs := []string{".", disabledDirectory}
	if kind == DataPack {
		subdirs = subdirs[:1]
	}
	for _, subdir := range subdirs {
		entries, err := fs.ReadDir(root.FS(), subdir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if strings.HasPrefix(entry.Name(), ".tomori-") || !kind.Accepts(entry.Name(), entry.IsDir()) || entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			rel := filepath.Join(subdir, entry.Name())
			info, err := root.Stat(rel)
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() && !info.IsDir() {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), ".disabled")
			item := Entry{File: name, Relative: rel, Name: strings.TrimSuffix(name, filepath.Ext(name)),
				Size: info.Size(), Modified: info.ModTime(), Directory: info.IsDir(),
				Enabled: subdir == "." && !strings.HasSuffix(entry.Name(), ".disabled")}
			if err := readMetadata(ctx, root, rel, kind, &item); err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				item.Problem = err.Error()
			}
			list = append(list, item)
		}
	}
	slices.SortStableFunc(list, func(a, b Entry) int { return fsutil.CompareFold(a.Name, b.Name) })
	return list, nil
}

func openEntry(gameDir string, kind Kind, entry Entry) (*os.Root, error) {
	dir, err := Directory(gameDir, kind)
	if err != nil {
		return nil, err
	}
	if err := fsutil.ValidateID(entry.File); err != nil {
		return nil, err
	}
	if entry.Relative != entry.File && entry.Relative != entry.File+".disabled" && entry.Relative != filepath.Join(disabledDirectory, entry.File) {
		return nil, errors.New(locales.InvalidResourcePath())
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	info, err := root.Lstat(entry.Relative)
	if err == nil && (info.Mode()&os.ModeSymlink != 0 || info.IsDir() != entry.Directory || !entry.Directory && !info.Mode().IsRegular()) {
		err = errors.New(locales.ResourceChangedRefreshTheList())
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

func Toggle(ctx context.Context, gameDir string, kind Kind, entry Entry) error {
	if kind == DataPack {
		return errors.New(locales.ManageDataPacksWithinAWorld())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := openEntry(gameDir, kind, entry)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	target := entry.File
	if entry.Enabled {
		if entry.Directory {
			if err := root.MkdirAll(disabledDirectory, 0o755); err != nil {
				return err
			}
			target = filepath.Join(disabledDirectory, entry.File)
		} else {
			target += ".disabled"
		}
	}
	if _, err := root.Lstat(target); !errors.Is(err, os.ErrNotExist) {
		if err != nil {
			return err
		}
		return fmt.Errorf(locales.AResourceWithThisNameAlreadyExists2(), entry.File)
	}
	return root.Rename(entry.Relative, target)
}

func Remove(ctx context.Context, gameDir string, kind Kind, entry Entry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := openEntry(gameDir, kind, entry)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return root.RemoveAll(entry.Relative)
}

func Hash(ctx context.Context, path string) (string, error) {
	f, err := fsutil.OpenRegular(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	return hashFile(ctx, f, nil)
}

var hashBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

func HashEntry(ctx context.Context, gameDir string, kind Kind, entry Entry) (string, error) {
	f, err := OpenEntry(gameDir, kind, entry)
	if err != nil {
		return "", err
	}
	defer f.Close()
	return hashFile(ctx, f, &entry)
}

func OpenEntry(gameDir string, kind Kind, entry Entry) (*os.File, error) {
	root, err := openEntry(gameDir, kind, entry)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := fsutil.OpenRootRegular(root, entry.Relative)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || entry.Directory || info.Size() != entry.Size || !info.ModTime().Equal(entry.Modified) || !sameResource(root, entry.Relative, info)) {
		err = errors.New(locales.ResourceChangedRefreshTheList())
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func hashFile(ctx context.Context, f *os.File, entry *Entry) (string, error) {
	before, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !before.Mode().IsRegular() || entry != nil && (entry.Directory || before.Size() != entry.Size || !before.ModTime().Equal(entry.Modified)) {
		return "", errors.New(locales.ResourceChangedRefreshTheList())
	}
	h := sha1.New()
	buffer := hashBuffers.Get().(*[]byte)
	_, err = io.CopyBuffer(h, fsutil.ContextReader(ctx, f), *buffer)
	hashBuffers.Put(buffer)
	if err != nil {
		return "", err
	}
	after, err := f.Stat()
	if err != nil {
		return "", err
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", errors.New(locales.ResourceChangedRefreshTheList())
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Import preserves existing files and returns individual failures for a batch.
func Import(ctx context.Context, gameDir string, kind Kind, paths []string, compatibility Compatibility) (int, error) {
	entries, err := Scan(ctx, gameDir, kind)
	if err != nil {
		return 0, err
	}
	activeIDs := make(map[string]string)
	for _, entry := range entries {
		if entry.Enabled && entry.ID != "" {
			activeIDs[entry.ID] = entry.File
		}
	}
	var problems []error
	count := 0
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		candidate, err := Inspect(ctx, path, kind)
		if err == nil && candidate.Directory && kind != ResourcePack && kind != ShaderPack {
			err = errors.New(locales.SelectResourceFiles())
		}
		if err == nil {
			err = candidate.Validate(compatibility)
		}
		if err == nil && kind == Mod && candidate.ID != "" && !strings.HasSuffix(filepath.Base(path), ".disabled") && activeIDs[candidate.ID] != "" {
			err = fmt.Errorf(locales.ModAlreadyExists(), activeIDs[candidate.ID])
		}
		if err == nil {
			var imported *Imported
			imported, err = importFile(ctx, gameDir, kind, path, "")
			if imported != nil {
				_ = imported.Close()
			}
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", filepath.Base(path), err))
		} else {
			count++
			if candidate.ID != "" && !strings.HasSuffix(filepath.Base(path), ".disabled") {
				activeIDs[candidate.ID] = filepath.Base(path)
			}
		}
	}
	return count, errors.Join(problems...)
}

type Imported struct {
	root *os.Root
	name string
	info os.FileInfo
}

func (f *Imported) Close() error { return f.root.Close() }

func (f *Imported) FileInfo() os.FileInfo { return f.info }

func (f *Imported) Undo() error {
	info, err := f.root.Lstat(f.name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(f.info, info) || info.Size() != f.info.Size() || !info.ModTime().Equal(f.info.ModTime()) {
		return errors.New(locales.ResourceChangedRefreshTheList())
	}
	return f.root.Remove(f.name)
}

func ImportVerified(ctx context.Context, gameDir string, kind Kind, path, expected string) (*Imported, error) {
	decoded, err := hex.DecodeString(expected)
	if err != nil || len(decoded) != sha1.Size {
		return nil, errors.New(locales.InvalidResourceChecksum())
	}
	return importFile(ctx, gameDir, kind, path, expected)
}

func importFile(ctx context.Context, gameDir string, kind Kind, path, expected string) (*Imported, error) {
	name := filepath.Base(path)
	if err := fsutil.ValidateID(name); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !kind.Accepts(name, info.IsDir()) {
		return nil, errors.New(locales.UnsupportedFileType())
	}
	dir, err := Directory(gameDir, kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	for _, existing := range []string{name, name + ".disabled", filepath.Join(disabledDirectory, name)} {
		if _, err := root.Lstat(existing); err == nil {
			_ = root.Close()
			return nil, errors.New(locales.AResourceWithThisNameAlreadyExists())
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = root.Close()
			return nil, err
		}
	}
	if info.IsDir() {
		source, err := os.OpenRoot(path)
		if err != nil {
			_ = root.Close()
			return nil, err
		}
		stage, err := os.MkdirTemp(dir, ".tomori-resource-*")
		if err == nil {
			var target *os.Root
			target, err = os.OpenRoot(stage)
			if err == nil {
				copier := fsutil.NewTreeCopier(ctx, target, nil, nil)
				err = copier.Tree(source, nil)
				if closeErr := copier.Close(); err == nil {
					err = closeErr
				}
				err = errors.Join(err, target.Close())
			}
			if err == nil {
				err = os.Rename(stage, filepath.Join(dir, name))
			}
			if err != nil {
				_ = os.RemoveAll(stage)
			}
		}
		_ = source.Close()
		_ = root.Close()
		return nil, err
	}
	_ = root.Close()
	in, err := fsutil.OpenRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = in.Close() }()
	info, err = in.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New(locales.SelectResourceFiles())
	}
	root, err = os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	keepRoot := false
	defer func() {
		if !keepRoot {
			_ = root.Close()
		}
	}()
	base := strings.TrimSuffix(name, ".disabled")
	for _, existing := range []string{base, base + ".disabled", filepath.Join(disabledDirectory, base)} {
		if _, err := root.Lstat(existing); err == nil {
			return nil, errors.New(locales.AResourceWithThisNameAlreadyExists())
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	out, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	hash := sha1.New()
	var writer io.Writer = out
	if expected != "" {
		writer = io.MultiWriter(out, hash)
	}
	buffer := hashBuffers.Get().(*[]byte)
	_, err = io.CopyBuffer(struct{ io.Writer }{writer}, fsutil.ContextReader(ctx, in), *buffer)
	hashBuffers.Put(buffer)
	if err == nil && expected != "" && !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		err = fmt.Errorf(locales.FileVerificationFailed(), name)
	}
	if err == nil {
		err = ctx.Err()
	}
	published, statErr := out.Stat()
	if err == nil {
		err = statErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		if published != nil {
			err = errors.Join(err, (&Imported{root: root, name: name, info: published}).Undo())
		}
		return nil, err
	}
	keepRoot = true
	return &Imported{root: root, name: name, info: published}, nil
}
