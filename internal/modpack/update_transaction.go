package modpack

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type updateOperation struct {
	index          int
	old, target    string
	next           InstalledFile
	previous       updateCopy
	staged         updateCopy
	backup         os.FileInfo
	published      updateCopy
	backedUp       bool
	publicationErr bool
}

type updateSet struct {
	root, stage *os.Root
	name        string
	directory   string
	stamp, base os.FileInfo
	files       []updateOperation
	permit      func(context.Context) (func(), error)
}

type UpdateTransaction struct {
	*updateSet
	plan        *UpdatePlan
	control     *updateSet
	finished    bool
	rollbackErr error
}

func newUpdateSet(root *os.Root, expected os.FileInfo, prefix string, permit func(context.Context) (func(), error)) (*updateSet, error) {
	if err := updateRootCurrent(root, expected); err != nil {
		return nil, err
	}
	held, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	name := prefix + rand.Text()
	return &updateSet{root: held, name: name, directory: filepath.Clean(filepath.Join(root.Name(), name)), base: expected, permit: permit}, nil
}

func (t *updateSet) makeStage() error {
	if err := t.root.Mkdir(t.name, 0o700); err != nil {
		return err
	}
	var err error
	t.stamp, err = t.root.Lstat(t.name)
	if err != nil {
		return err
	}
	if !t.stamp.IsDir() {
		t.stamp = nil
		return updateChanged()
	}
	t.stage, err = t.root.OpenRoot(t.name)
	if err != nil {
		return err
	}
	if err := t.current(); err != nil {
		return err
	}
	for _, name := range []string{"next", "previous"} {
		if err := t.stage.Mkdir(name, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (p *UpdatePlan) Apply(ctx context.Context, root, candidate *os.Root, choices map[string]UpdateChoice, permit func(context.Context) (func(), error), progress func(int, int), beforeWrite func(string) error) (_ *UpdateTransaction, err error) {
	if len(choices) > p.Len() {
		return nil, updateChanged()
	}
	choices = maps.Clone(choices)
	selected, err := p.selections(choices)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := updateRootCurrent(root, p.root); err != nil {
		return nil, err
	}
	space, err := p.AssessSpace(ctx, root, choices)
	if err != nil {
		return nil, err
	}
	if err := space.Check(); err != nil {
		return nil, err
	}
	set, err := newUpdateSet(root, p.root, ".tomori-pack-update-", permit)
	if err != nil {
		return nil, err
	}
	t := &UpdateTransaction{updateSet: set, plan: p}
	defer func() {
		if err != nil {
			err = errors.Join(err, t.Close())
		}
	}()
	oldPaths, targets := make(map[string]bool, len(selected)), make(map[string]bool, len(selected))
	for _, index := range selected {
		entry := p.Entry(index)
		if candidate == nil && entry.Next.Path != "" {
			return nil, invalidOrigin()
		}
		for _, name := range []string{entry.Path, entry.Target} {
			if err := updateContentPath(name); err != nil {
				return nil, err
			}
		}
		file := updateOperation{index: index, target: entry.Target, next: entry.Next}
		if key := strings.ToLower(file.target); targets[key] {
			return nil, updateChanged()
		} else {
			targets[key] = true
		}
		current := p.files[index].current
		if current.info != nil {
			if err := updateContentPath(current.relative); err != nil {
				return nil, err
			}
			file.old = current.relative
			if key := strings.ToLower(file.old); oldPaths[key] {
				return nil, updateChanged()
			} else {
				oldPaths[key] = true
			}
			file.previous = updateCopy{info: current.info, hash: current.hash, size: current.info.Size()}
		}
		t.files = append(t.files, file)
	}
	if err := t.makeStage(); err != nil {
		return nil, err
	}
	for _, name := range []string{"previous-origin", "next-origin"} {
		if err := t.stage.Mkdir(name, 0o700); err != nil {
			return nil, err
		}
	}
	if err := t.step(ctx, func() error {
		for _, item := range []struct {
			name   string
			origin *Origin
		}{{"previous-origin", p.previous}, {"next-origin", p.next}} {
			r, err := t.stage.OpenRoot(item.name)
			if err != nil {
				return err
			}
			writeErr := item.origin.Write(ctx, r)
			if err := errors.Join(writeErr, r.Close()); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	staged := selected[:0]
	for i, file := range t.files {
		if file.next.Path != "" {
			staged = append(staged, i)
		}
	}
	err = updateWorkers(ctx, len(staged), permit, progress, func(ctx context.Context, index int) error {
		i := staged[index]
		file := &t.files[i]
		next := file.next
		var err error
		file.staged, err = copyUpdateFile(ctx, candidate, next.Path, t.stage, "next/"+strconv.Itoa(i), next.SHA1, next.Size, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := t.step(ctx, func() error {
		return t.writeManifest(ctx, func(i int) UpdateEntry { return p.Entry(t.files[i].index) })
	}); err != nil {
		return nil, err
	}
	if err := p.Validate(ctx, t.root, choices, permit); err != nil {
		return nil, err
	}
	if beforeWrite != nil {
		if err := beforeWrite(t.BackupPath()); err != nil {
			return nil, err
		}
	}
	for i := range t.files {
		if err := t.step(ctx, func() error { return t.applyFile(ctx, i) }); err != nil {
			return nil, err
		}
	}
	if err := t.Validate(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *UpdateTransaction) BackupPath() string {
	if t.updateSet == nil {
		return ""
	}
	return t.directory
}

func (t *updateSet) current() error {
	if t.root == nil || t.stamp == nil {
		return updateChanged()
	}
	if err := updateRootCurrent(t.root, t.base); err != nil {
		return err
	}
	info, err := t.root.Lstat(t.name)
	if err != nil {
		return err
	}
	if !info.IsDir() || !os.SameFile(info, t.stamp) {
		return updateChanged()
	}
	if t.stage != nil {
		held, err := t.stage.Stat(".")
		if err != nil {
			return err
		}
		if !os.SameFile(held, t.stamp) {
			return updateChanged()
		}
	}
	return nil
}

func (t *updateSet) step(ctx context.Context, work func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var release func()
	if t.permit != nil {
		var err error
		release, err = t.permit(ctx)
		if err != nil {
			return err
		}
		if release != nil {
			defer release()
		}
	}
	if err := t.current(); err != nil {
		return err
	}
	return work()
}

func (t *updateSet) applyFile(ctx context.Context, i int) error {
	file := &t.files[i]
	if file.old != "" {
		current, err := updateHash(ctx, t.root, file.old, file.previous.info)
		if err != nil {
			return err
		}
		if current.hash != file.previous.hash {
			return updateChanged()
		}
		backup := path.Join(t.name, "previous", strconv.Itoa(i))
		if err := t.root.Rename(file.old, backup); err != nil {
			return err
		}
		file.backedUp = true
		file.backup, err = t.root.Lstat(backup)
		if err != nil {
			return err
		}
		if !sameUpdateFile(file.previous.info, file.backup) {
			return updateChanged()
		}
	}
	if file.next.Path == "" {
		return nil
	}
	source := path.Join(t.name, "next", strconv.Itoa(i))
	var err error
	file.published, err = publishUpdateFile(ctx, t.root, source, file.target, file.staged)
	file.publicationErr = err != nil
	return err
}

func (t *UpdateTransaction) Validate(ctx context.Context) error {
	if t.finished || t.updateSet == nil || t.root == nil {
		return errors.New(locales.ModpackUpdateTransactionHasEnded())
	}
	if err := t.validate(ctx); err != nil {
		return err
	}
	return t.control.validate(ctx)
}

func (t *updateSet) validate(ctx context.Context) error {
	if t == nil {
		return ctx.Err()
	}
	if err := t.current(); err != nil {
		return err
	}
	return updateWorkers(ctx, len(t.files), t.permit, nil, func(ctx context.Context, i int) error {
		file := &t.files[i]
		if file.backedUp {
			current, err := updateHash(ctx, t.stage, "previous/"+strconv.Itoa(i), file.previous.info)
			if err != nil {
				return err
			}
			if current.hash != file.previous.hash {
				return updateChanged()
			}
		}
		if file.published.info != nil {
			current, err := updateHash(ctx, t.root, file.target, file.published.info)
			if err != nil {
				return err
			}
			if current.hash != file.published.hash {
				return updateChanged()
			}
		} else if file.next.Path != "" {
			return updateChanged()
		}
		if file.old != "" && file.old != file.target || file.next.Path == "" {
			name := file.old
			if name == "" {
				name = file.target
			}
			_, err := t.root.Lstat(name)
			if !errors.Is(err, os.ErrNotExist) {
				return errors.Join(err, updateChanged())
			}
		}
		return nil
	})
}

func (t *UpdateTransaction) Commit(ctx context.Context) error {
	if err := t.Validate(ctx); err != nil {
		return err
	}
	if t.control != nil {
		if err := t.control.step(ctx, func() error {
			if err := t.control.stage.RemoveAll("next"); err != nil {
				return err
			}
			return fsutil.WriteRootAtomic(ctx, t.control.stage, "ready", []byte("1\n"))
		}); err != nil {
			return err
		}
	}
	if err := t.step(ctx, func() error {
		if err := t.stage.RemoveAll("next"); err != nil {
			return err
		}
		return fsutil.WriteRootAtomic(ctx, t.stage, "committed", []byte("1\n"))
	}); err != nil {
		return err
	}
	t.finished = true
	return nil
}

func (t *UpdateTransaction) Rollback() error {
	if t.finished || t.updateSet == nil || t.root == nil {
		return t.rollbackErr
	}
	t.finished = true
	t.rollbackErr = errors.Join(t.control.rollback(), t.rollback())
	return t.rollbackErr
}

func (t *updateSet) rollback() error {
	if t == nil || t.root == nil {
		return nil
	}
	var problems []error
	for i := len(t.files) - 1; i >= 0; i-- {
		file := &t.files[i]
		if !file.backedUp && file.published.info == nil {
			continue
		}
		err := t.step(context.Background(), func() error {
			var expected updateCopy
			backup := path.Join(t.name, "previous", strconv.Itoa(i))
			if file.backedUp {
				if file.backup == nil {
					return updateChanged()
				}
				var err error
				expected, err = updateHash(context.Background(), t.root, backup, file.backup)
				if err != nil {
					return err
				}
				if sameUpdateFile(file.previous.info, file.backup) && expected.hash != file.previous.hash {
					return updateChanged()
				}
			}
			if file.published.info != nil {
				if !file.publicationErr {
					current, err := updateHash(context.Background(), t.root, file.target, file.published.info)
					if err != nil && !errors.Is(err, os.ErrNotExist) {
						return err
					}
					if err == nil && current.hash != file.published.hash {
						return updateChanged()
					}
				}
				if err := removeUpdateFile(t.root, file.target, file.published.info); err != nil {
					return err
				}
			}
			if !file.backedUp {
				return nil
			}
			restored, err := publishUpdateFile(context.Background(), t.root, backup, file.old, expected)
			if err != nil && restored.info != nil {
				err = errors.Join(err, removeUpdateFile(t.root, file.old, restored.info))
			}
			return err
		})
		if err != nil {
			problems = append(problems, err)
		}
	}
	if len(problems) == 0 && t.stamp != nil {
		if err := t.current(); err != nil {
			problems = append(problems, err)
		} else if err := t.root.RemoveAll(t.name); err != nil {
			problems = append(problems, err)
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf(locales.CouldNotRestoreModpackFilesBackupAt(), t.directory, errors.Join(problems...))
	}
	return nil
}

func (t *UpdateTransaction) Close() error {
	err := t.Rollback()
	err = errors.Join(err, t.control.close(), t.close())
	t.plan, t.control = nil, nil
	return err
}

func (t *updateSet) close() error {
	if t == nil {
		return nil
	}
	var err error
	if t.stage != nil {
		err = errors.Join(err, t.stage.Close())
		t.stage = nil
	}
	if t.root != nil {
		err = errors.Join(err, t.root.Close())
		t.root = nil
	}
	t.files = nil
	return err
}

type backupHeader struct {
	Schema    int       `json:"schema"`
	CreatedAt time.Time `json:"createdAt"`
	FileCount int       `json:"fileCount"`
}

type backupRecord struct {
	Old        string      `json:"old,omitempty"`
	Target     string      `json:"target"`
	BeforeSHA1 string      `json:"beforeSHA1,omitempty"`
	BeforeSize int64       `json:"beforeSize,omitempty"`
	BeforeMode uint32      `json:"beforeMode,omitempty"`
	AfterMode  *uint32     `json:"afterMode,omitempty"`
	Entry      UpdateEntry `json:"entry"`
}

func (t *updateSet) writeManifest(ctx context.Context, entry func(int) UpdateEntry) error {
	header := backupHeader{1, time.Now().UTC(), len(t.files)}
	encoded, err := json.Marshal(header)
	if err != nil {
		return err
	}
	return fsutil.WriteRootAtomicStream(ctx, t.stage, "manifest.json", func(out io.Writer) error {
		w := bufio.NewWriterSize(&originWriter{out: out}, 64<<10)
		if _, err := w.WriteString("{\"header\":" + string(encoded) + ",\"files\":[\n"); err != nil {
			return err
		}
		encoder := json.NewEncoder(w)
		for i, file := range t.files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if i > 0 {
				if err := w.WriteByte(','); err != nil {
					return err
				}
			}
			record := backupRecord{Old: file.old, Target: file.target, BeforeSHA1: file.previous.hash, BeforeSize: file.previous.size, Entry: entry(i)}
			if file.previous.info != nil {
				record.BeforeMode = uint32(file.previous.info.Mode().Perm())
			}
			if file.staged.info != nil {
				mode := uint32(file.staged.info.Mode().Perm())
				record.AfterMode = &mode
			}
			if err := encoder.Encode(record); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("]}\n"); err != nil {
			return err
		}
		return w.Flush()
	})
}
