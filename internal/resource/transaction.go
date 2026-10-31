package resource

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type Change struct {
	Kind             Kind
	Previous         *Entry
	PreviousSHA1     string
	Path, SHA1       string
	Disabled         bool
	PreservePrevious bool
}

type changeRoot struct {
	root, stage *os.Root
	path, name  string
}

type fileChange struct {
	root                    *changeRoot
	old, backup, next, name string
	hash                    string
	previous, staged        os.FileInfo
	published, preserved    *Imported
	backedUp                bool
	preservePrevious        bool
}

type Transaction struct {
	roots    map[Kind]*changeRoot
	files    []fileChange
	finished bool
}

func ApplyChanges(ctx context.Context, gameDir string, changes []Change) (_ *Transaction, err error) {
	if len(changes) == 0 || len(changes) > 1000 {
		return nil, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	t := &Transaction{roots: make(map[Kind]*changeRoot)}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Close())
		}
	}()
	seen := make(map[string]bool, len(changes)*2)
	for _, change := range changes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !change.Kind.Valid() || !validHash(change.SHA1) || change.Previous != nil && !validHash(change.PreviousSHA1) {
			return nil, errors.New(locales.InvalidResourceChecksum())
		}
		name := strings.TrimSuffix(filepath.Base(change.Path), ".disabled")
		if err := fsutil.ValidateID(name); err != nil {
			return nil, err
		}
		if !change.Kind.Accepts(name, false) {
			return nil, errors.New(locales.UnsupportedFileType())
		}
		r, err := t.openRoot(gameDir, change.Kind)
		if err != nil {
			return nil, err
		}
		file := fileChange{root: r, name: name, next: filepath.Join(r.name, "next", name), preservePrevious: change.PreservePrevious}
		if change.Previous != nil && !change.Previous.Enabled {
			file.preservePrevious = false
		}
		if change.Previous == nil && change.Disabled && change.Kind != DataPack {
			file.name += ".disabled"
		}
		if old := change.Previous; old != nil {
			validPath := old.Relative == old.File
			if change.Kind != DataPack {
				validPath = old.Enabled && validPath || !old.Enabled && (old.Relative == old.File+".disabled" || old.Relative == filepath.Join(disabledDirectory, old.File))
			}
			if old.Directory || !validPath {
				return nil, errors.New(locales.InvalidResourcePath())
			}
			if err := fsutil.ValidateID(old.File); err != nil {
				return nil, err
			}
			file.old, file.backup = old.Relative, filepath.Join(r.name, "previous", old.Relative)
			file.hash = change.PreviousSHA1
			if !old.Enabled && change.Kind != DataPack {
				file.name += ".disabled"
				if strings.HasPrefix(old.Relative, disabledDirectory+string(filepath.Separator)) {
					file.name = filepath.Join(disabledDirectory, name)
				}
			}
			before, err := r.root.Lstat(file.old)
			if err != nil {
				return nil, err
			}
			if !before.Mode().IsRegular() {
				return nil, errors.New(locales.SelectResourceFiles())
			}
			in, err := r.root.Open(file.old)
			if err != nil {
				return nil, err
			}
			hash, hashErr := hashFile(ctx, in, old)
			file.previous, err = in.Stat()
			_ = in.Close()
			if hashErr != nil {
				return nil, hashErr
			}
			if err != nil {
				return nil, err
			}
			if !strings.EqualFold(hash, change.PreviousSHA1) || !sameResource(r.root, file.old, file.previous) {
				return nil, errors.New(locales.ResourceChangedRefreshTheList())
			}
			key := string(change.Kind) + ":old:" + strings.ToLower(file.old)
			if seen[key] {
				return nil, errors.New(locales.ResourceUpdateContainsDuplicateFiles())
			}
			seen[key] = true
		}
		key := string(change.Kind) + ":new:" + strings.ToLower(name)
		if seen[key] {
			return nil, errors.New(locales.ResourceUpdateContainsDuplicateFiles())
		}
		seen[key] = true
		if err := r.available(name, file.old); err != nil {
			return nil, err
		}
		in, err := fsutil.OpenRegular(change.Path)
		if err != nil {
			return nil, err
		}
		file.staged, err = copyVerified(ctx, r.root, file.next, in, change.SHA1)
		_ = in.Close()
		if err != nil {
			return nil, err
		}
		t.files = append(t.files, file)
	}
	for i := range t.files {
		file := &t.files[i]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !file.root.current() {
			return nil, errors.New(locales.ResourceChangedRefreshTheList())
		}
		if file.old != "" {
			if !sameResource(file.root.root, file.old, file.previous) {
				return nil, errors.New(locales.ResourceChangedRefreshTheList())
			}
			if err := file.root.root.MkdirAll(filepath.Dir(file.backup), 0o700); err != nil {
				return nil, err
			}
			if err := file.root.root.Rename(file.old, file.backup); err != nil {
				return nil, err
			}
			file.backedUp = true
			if !sameResource(file.root.root, file.backup, file.previous) {
				return nil, errors.New(locales.ResourceChangedRefreshTheList())
			}
		}
	}
	for i := range t.files {
		file := &t.files[i]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !file.root.current() || !sameResource(file.root.root, file.next, file.staged) {
			return nil, errors.New(locales.ResourceChangedRefreshTheList())
		}
		if err := file.root.root.MkdirAll(filepath.Dir(file.name), 0o755); err != nil {
			return nil, err
		}
		if err := file.root.root.Link(file.next, file.name); err == nil {
			file.published = &Imported{root: file.root.root, name: file.name, info: file.staged}
		} else {
			in, openErr := file.root.root.Open(file.next)
			if openErr != nil {
				return nil, errors.Join(err, openErr)
			}
			info, copyErr := copyVerified(ctx, file.root.root, file.name, in, changes[i].SHA1)
			_ = in.Close()
			if info != nil {
				file.published = &Imported{root: file.root.root, name: file.name, info: info}
			}
			if copyErr != nil {
				return nil, errors.Join(err, copyErr)
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

func validHash(hash string) bool {
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == sha1.Size
}

func sameResource(root *os.Root, path string, expected os.FileInfo) bool {
	info, err := root.Lstat(path)
	return err == nil && info.Mode().IsRegular() && os.SameFile(info, expected) && info.Size() == expected.Size() && info.ModTime().Equal(expected.ModTime())
}

func copyVerified(ctx context.Context, root *os.Root, path string, in *os.File, expected string) (os.FileInfo, error) {
	info, err := in.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New(locales.SelectResourceFiles())
	}
	out, err := root.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	hash := sha1.New()
	buffer := hashBuffers.Get().(*[]byte)
	_, err = io.CopyBuffer(struct{ io.Writer }{io.MultiWriter(out, hash)}, fsutil.ContextReader(ctx, in), *buffer)
	hashBuffers.Put(buffer)
	if err == nil && !strings.EqualFold(expected, hex.EncodeToString(hash.Sum(nil))) {
		err = fmt.Errorf(locales.FileVerificationFailed(), filepath.Base(path))
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = out.Sync()
	}
	published, statErr := out.Stat()
	if err == nil {
		err = statErr
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	return published, err
}

func (t *Transaction) openRoot(gameDir string, kind Kind) (*changeRoot, error) {
	if r := t.roots[kind]; r != nil {
		return r, nil
	}
	path, err := Directory(gameDir, kind)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	r := &changeRoot{root: root, path: path, name: ".tomori-update-" + rand.Text()}
	if err := root.Mkdir(r.name, 0o700); err != nil {
		_ = root.Close()
		return nil, err
	}
	t.roots[kind] = r
	r.stage, err = root.OpenRoot(r.name)
	if err == nil {
		err = r.stage.Mkdir("next", 0o700)
	}
	return r, err
}

func (r *changeRoot) current() bool {
	original, err := r.root.Stat(".")
	current, statErr := os.Stat(r.path)
	return err == nil && statErr == nil && os.SameFile(original, current)
}

func (r *changeRoot) available(name, old string) error {
	for _, path := range []string{name, name + ".disabled", filepath.Join(disabledDirectory, name)} {
		if path == old {
			continue
		}
		if info, err := r.root.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			if err != nil {
				return err
			}
			if old != "" {
				previous, oldErr := r.root.Lstat(old)
				if oldErr == nil && os.SameFile(info, previous) {
					continue
				}
			}
			return errors.New(locales.AResourceWithThisNameAlreadyExists())
		}
	}
	return nil
}

func (t *Transaction) Validate() error {
	if t.finished {
		return errors.New(locales.ResourceUpdateTransactionHasEnded())
	}
	for _, file := range t.files {
		if !file.root.current() || file.published == nil || !sameResource(file.root.root, file.name, file.published.info) || file.backedUp && !sameResource(file.root.root, file.backup, file.previous) {
			return errors.New(locales.ResourceChangedRefreshTheList())
		}
	}
	return nil
}

func (t *Transaction) Commit() error {
	if t.finished {
		return errors.New(locales.ResourceUpdateTransactionHasEnded())
	}
	for _, file := range t.files {
		if file.backedUp && !sameResource(file.root.root, file.backup, file.previous) {
			return fmt.Errorf(locales.CouldNotCleanResourceBackupAt(), filepath.Join(file.root.path, file.root.name, "previous"), errors.New(locales.ResourceChangedRefreshTheList()))
		}
	}
	for i := range t.files {
		file := &t.files[i]
		if !file.backedUp || !file.preservePrevious {
			continue
		}
		if err := file.root.root.Link(file.backup, file.old+".disabled"); err != nil {
			return fmt.Errorf(locales.CouldNotPreserveOldResource(), err)
		}
		file.preserved = &Imported{root: file.root.root, name: file.old + ".disabled", info: file.previous}
	}
	t.finished = true
	return t.clean()
}

func (t *Transaction) Rollback() error {
	if t.finished {
		return nil
	}
	t.finished = true
	var problems []error
	for i := len(t.files) - 1; i >= 0; i-- {
		file := &t.files[i]
		if file.preserved != nil {
			if err := file.preserved.Undo(); err != nil {
				problems = append(problems, err)
			}
		}
		if file.published != nil {
			if err := file.published.Undo(); err != nil {
				problems = append(problems, err)
			}
		}
		if file.backedUp {
			if err := file.restore(); err != nil {
				problems = append(problems, fmt.Errorf(locales.CouldNotRestoreResourceBackupAt(), filepath.Join(file.root.path, file.root.name, "previous"), err))
			}
		}
	}
	if len(problems) == 0 {
		return t.clean()
	}
	return errors.Join(problems...)
}

func (f *fileChange) restore() error {
	if !sameResource(f.root.root, f.backup, f.previous) {
		return errors.New(locales.ResourceChangedRefreshTheList())
	}
	if err := f.root.root.Link(f.backup, f.old); err == nil || sameResource(f.root.root, f.old, f.previous) {
		return nil
	}
	in, err := f.root.root.Open(f.backup)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := copyVerified(context.Background(), f.root.root, f.old, in, f.hash)
	if err != nil && info != nil {
		err = errors.Join(err, (&Imported{root: f.root.root, name: f.old, info: info}).Undo())
	}
	return err
}

func (t *Transaction) clean() error {
	var problems []error
	for _, r := range t.roots {
		if r.stage == nil {
			continue
		}
		staged, err := r.stage.Stat(".")
		current, statErr := r.root.Lstat(r.name)
		if err == nil && statErr == nil && os.SameFile(staged, current) {
			err = r.root.RemoveAll(r.name)
		} else {
			err = errors.Join(err, statErr, errors.New(locales.ResourceChangedRefreshTheList()))
		}
		if err != nil {
			problems = append(problems, fmt.Errorf(locales.CouldNotCleanResourceBackupAt(), filepath.Join(r.path, r.name), err))
		}
	}
	return errors.Join(problems...)
}

func (t *Transaction) Close() error {
	err := t.Rollback()
	for _, r := range t.roots {
		if r.stage != nil {
			err = errors.Join(err, r.stage.Close())
		}
		err = errors.Join(err, r.root.Close())
	}
	t.files, t.roots = nil, nil
	return err
}
