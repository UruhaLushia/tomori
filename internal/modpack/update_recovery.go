package modpack

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type RecoveryPlan struct {
	Info            BackupInfo
	mu              sync.Mutex
	content, config *updateSet
	files, controls []recoveryFile
	finished        bool
}

type recoverySpec struct {
	old, target, hash, nextHash string
	size, nextSize              int64
	mode, nextMode              uint32
	hasNext, hasNextMode        bool
}

type recoveryFile struct {
	recoverySpec
	backup, oldLive, targetLive updateCopy
}

func unfinishedUpdate() error {
	return errors.New(locales.InterruptedUpdateChangedCheckAgain())
}

func recoveryOpen(root *os.Root, name string, permit func(context.Context) (func(), error)) (*updateSet, error) {
	base, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	set, err := newUpdateSet(root, base, ".tomori-pack-update-", permit)
	if err != nil {
		return nil, err
	}
	set.name, set.directory = name, filepath.Join(root.Name(), name)
	set.stage, set.stamp, err = openBackupRoot(root, name)
	if err != nil {
		return nil, errors.Join(err, set.close())
	}
	return set, nil
}

func recoverySpecs(records []backupRecord) ([]recoveryFile, error) {
	files := make([]recoveryFile, len(records))
	paths := make(map[string]int, len(records))
	for i, record := range records {
		next := record.Entry.Next
		spec := recoverySpec{old: record.Old, target: record.Target, hash: record.BeforeSHA1, size: record.BeforeSize, mode: record.BeforeMode, nextHash: next.SHA1, nextSize: next.Size, hasNext: next.Path != ""}
		if record.AfterMode != nil {
			spec.hasNextMode, spec.nextMode = true, *record.AfterMode
		}
		for _, name := range []string{spec.old, spec.target} {
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if owner, found := paths[key]; found && owner != i {
				return nil, invalidBackup()
			}
			paths[key] = i
		}
		files[i].recoverySpec = spec
	}
	return files, nil
}

func recoveryConfigSpecs(records []backupControlRecord) []recoveryFile {
	files := make([]recoveryFile, len(records))
	for i, record := range records {
		spec := recoverySpec{target: record.Path, hash: record.BeforeSHA1, size: record.BeforeSize, mode: record.BeforeMode, nextHash: record.Next.SHA1, nextSize: record.Next.Size, hasNext: record.Next.Path != ""}
		if record.Old {
			spec.old = record.Path
		}
		if record.AfterMode != nil {
			spec.hasNextMode, spec.nextMode = true, *record.AfterMode
		}
		files[i].recoverySpec = spec
	}
	return files
}

func PrepareRecovery(ctx context.Context, content, metadata *os.Root, name string, permit func(context.Context) (func(), error), progress func(int, int)) (_ *RecoveryPlan, err error) {
	if !backupName(name, ".tomori-pack-update-") {
		return nil, invalidBackup()
	}
	p := &RecoveryPlan{}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.Close())
		}
	}()
	p.content, err = recoveryOpen(content, name, permit)
	if err != nil {
		return nil, err
	}
	if err := p.current(); err != nil {
		return nil, err
	}
	err = p.content.step(ctx, func() error {
		if err := updateRootCurrent(metadata, nil); err != nil {
			return err
		}
		p.Info, err = backupSummary(ctx, p.content.stage, name)
		p.Info.Interrupted = true
		if err != nil {
			return err
		}
		records, err := readBackupRecords(ctx, p.content.stage)
		if err != nil {
			return err
		}
		p.files, err = recoverySpecs(records)
		if err != nil {
			return err
		}
		control, controlName, recordsConfig, err := readUpdateControls(ctx, metadata, p.content.stage, false)
		if err != nil || control == nil {
			return err
		}
		defer control.Close()
		p.config, err = recoveryOpen(metadata, controlName, permit)
		if err == nil {
			p.controls = recoveryConfigSpecs(recordsConfig)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := p.inspect(ctx, progress, false); err != nil {
		return nil, err
	}
	return p, p.current()
}

func recoveryHash(ctx context.Context, root *os.Root, name string) (updateCopy, error) {
	if name == "" {
		return updateCopy{}, nil
	}
	blocked, missing, err := updateParents(root, name)
	if err != nil || blocked {
		return updateCopy{}, errors.Join(err, restoreConflict(name))
	}
	if missing {
		return updateCopy{}, nil
	}
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return updateCopy{}, nil
	}
	if err != nil {
		return updateCopy{}, err
	}
	return updateHash(ctx, root, name, info)
}

func recoveryMatches(file updateCopy, hash string, size int64, mode uint32, checkMode bool) bool {
	return file.info != nil && file.hash == hash && file.size == size && (!checkMode || uint32(file.info.Mode().Perm()) == mode)
}

func inspectRecovery(ctx context.Context, set *updateSet, index int, spec recoverySpec) (recoveryFile, error) {
	file := recoveryFile{recoverySpec: spec}
	var err error
	file.backup, err = recoveryHash(ctx, set.stage, backupFileName(index))
	if err != nil {
		return file, err
	}
	if file.backup.info != nil && (spec.old == "" || !recoveryMatches(file.backup, spec.hash, spec.size, spec.mode, true)) {
		return file, invalidBackup()
	}
	file.oldLive, err = recoveryHash(ctx, set.root, spec.old)
	if err != nil {
		return file, err
	}
	if spec.old == spec.target {
		file.targetLive = file.oldLive
	} else {
		file.targetLive, err = recoveryHash(ctx, set.root, spec.target)
		if err != nil {
			return file, err
		}
	}
	before := recoveryMatches(file.oldLive, spec.hash, spec.size, spec.mode, true)
	if file.backup.info == nil && spec.old != "" && !before {
		return file, restoreConflict(spec.old)
	}
	if file.oldLive.info != nil && spec.old != spec.target && !before {
		return file, restoreConflict(spec.old)
	}
	if file.targetLive.info != nil && (spec.old != spec.target || !before) {
		if !spec.hasNext || spec.old != "" && file.backup.info == nil || !recoveryMatches(file.targetLive, spec.nextHash, spec.nextSize, spec.nextMode, spec.hasNextMode) {
			return file, restoreConflict(spec.target)
		}
	}
	return file, nil
}

func sameRecoveryCopy(a, b updateCopy) bool {
	if a.info == nil || b.info == nil {
		return a.info == nil && b.info == nil
	}
	return a.hash == b.hash && a.size == b.size && sameUpdateFile(a.info, b.info)
}

func (p *RecoveryPlan) inspect(ctx context.Context, progress func(int, int), validate bool) error {
	return updateWorkers(ctx, len(p.files)+len(p.controls), p.content.permit, progress, func(ctx context.Context, i int) error {
		set, files := p.content, p.files
		if i >= len(p.files) {
			i -= len(p.files)
			set, files = p.config, p.controls
		}
		file, err := inspectRecovery(ctx, set, i, files[i].recoverySpec)
		if err != nil {
			return err
		}
		if validate {
			previous := files[i]
			if !sameRecoveryCopy(previous.backup, file.backup) || !sameRecoveryCopy(previous.oldLive, file.oldLive) || !sameRecoveryCopy(previous.targetLive, file.targetLive) {
				return restoreConflict(file.target)
			}
		} else {
			files[i] = file
		}
		return nil
	})
}

func (p *RecoveryPlan) current() error {
	if p.finished || p.content == nil || p.content.root == nil {
		return unfinishedUpdate()
	}
	if err := p.content.current(); err != nil {
		return err
	}
	if p.config != nil {
		if err := p.config.current(); err != nil {
			return err
		}
	}
	for _, name := range []string{"committed", "recovered", "restored.json"} {
		if _, err := p.content.stage.Lstat(name); !errors.Is(err, os.ErrNotExist) {
			return errors.Join(err, unfinishedUpdate())
		}
	}
	return nil
}

func (p *RecoveryPlan) AssessSpace(ctx context.Context) (storage.Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.assessSpace(ctx)
}

func (p *RecoveryPlan) assessSpace(ctx context.Context) (storage.Report, error) {
	if err := p.current(); err != nil {
		return storage.Report{}, err
	}
	var needs []storage.Requirement
	for _, item := range []struct {
		set   *updateSet
		files []recoveryFile
	}{{p.content, p.files}, {p.config, p.controls}} {
		if item.set == nil {
			continue
		}
		bytes := uint64(64 << 10)
		for _, file := range item.files {
			if file.backup.info != nil && !recoveryMatches(file.oldLive, file.hash, file.size, file.mode, true) {
				bytes = storage.Add(bytes, uint64(file.size)*2)
			}
		}
		needs = append(needs, storage.Requirement{Directory: item.set.root.Name(), Bytes: bytes})
	}
	return storage.Assess(ctx, needs)
}

func (p *RecoveryPlan) Apply(ctx context.Context, progress func(int, int)) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	space, err := p.assessSpace(ctx)
	if err != nil {
		return "", err
	}
	if err := space.Check(); err != nil {
		return "", err
	}
	if err := p.inspect(ctx, nil, true); err != nil {
		return "", err
	}
	done, total := 0, len(p.controls)+len(p.files)
	var last time.Time
	for _, item := range []struct {
		set   *updateSet
		files []recoveryFile
	}{{p.config, p.controls}, {p.content, p.files}} {
		for i, v := range slices.Backward(item.files) {
			if err := item.set.step(ctx, func() error { return recoverFile(ctx, item.set, i, v) }); err != nil {
				return "", err
			}
			done++
			if progress != nil && (done == total || time.Since(last) >= 100*time.Millisecond) {
				last = time.Now()
				progress(done, total)
			}
		}
	}
	if err := p.verifyRecovered(ctx); err != nil {
		return "", err
	}
	if err := p.current(); err != nil {
		return "", err
	}
	if err := p.content.step(ctx, func() error { return fsutil.WriteRootAtomic(ctx, p.content.stage, "recovered", []byte("1\n")) }); err != nil {
		return "", err
	}
	p.finished = true
	return p.content.directory, nil
}

func (p *RecoveryPlan) verifyRecovered(ctx context.Context) error {
	return updateWorkers(ctx, len(p.files)+len(p.controls), p.content.permit, nil, func(ctx context.Context, i int) error {
		set, files := p.content, p.files
		if i >= len(p.files) {
			i -= len(p.files)
			set, files = p.config, p.controls
		}
		file := files[i]
		if file.old != "" {
			live, err := recoveryHash(ctx, set.root, file.old)
			if err != nil {
				return err
			}
			if !recoveryMatches(live, file.hash, file.size, file.mode, true) {
				return restoreConflict(file.old)
			}
		}
		if file.old != file.target {
			return restoreAbsent(set.root, file.target)
		}
		return nil
	})
}

func recoverFile(ctx context.Context, set *updateSet, index int, file recoveryFile) error {
	restore := file.backup.info != nil && !recoveryMatches(file.oldLive, file.hash, file.size, file.mode, true)
	var staged updateCopy
	var err error
	name := path.Join("recovery", strconv.Itoa(index))
	if restore {
		if err := updateEnsureParents(set.stage, name); err != nil {
			return err
		}
		if info, err := set.stage.Lstat(name); err == nil {
			if !info.Mode().IsRegular() {
				return invalidBackup()
			}
			if err := removeUpdateFile(set.stage, name, info); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		staged, err = copyUpdateFile(ctx, set.stage, backupFileName(index), set.stage, name, file.hash, file.size, file.backup.info)
		if err != nil {
			return err
		}
	}
	if file.targetLive.info != nil && (file.old != file.target || restore) {
		if err := removeUpdateFile(set.root, file.target, file.targetLive.info); err != nil {
			return err
		}
	}
	if restore {
		published, err := publishUpdateFile(ctx, set.root, path.Join(set.name, name), file.old, staged)
		if err != nil {
			if published.info != nil {
				err = errors.Join(err, removeUpdateFile(set.root, file.old, published.info))
			}
			return err
		}
		return removeUpdateFile(set.stage, name, staged.info)
	}
	return nil
}

func (p *RecoveryPlan) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := errors.Join(p.config.close(), p.content.close())
	p.config, p.content, p.files, p.controls = nil, nil, nil, nil
	return err
}
