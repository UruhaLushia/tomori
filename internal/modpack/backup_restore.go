package modpack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type RestorePlan struct {
	Info                   BackupInfo
	BeforeWrite            func(string) error
	mu                     sync.Mutex
	content, config        *updateSet
	files, controls        []updateOperation
	sources, configSources []updateCopy
	origins                [2]updateCopy
	manifestBytes          [2]int64
	finished               bool
}

func restoreConflict(name string) error {
	return fmt.Errorf(locales.FileChangedCannotRestoreDirectly(), name)
}

func PrepareRestore(ctx context.Context, content, metadata *os.Root, name string, permit func(context.Context) (func(), error), progress func(int, int)) (_ *RestorePlan, err error) {
	if !backupName(name, ".tomori-pack-update-") {
		return nil, invalidBackup()
	}
	if err := updateRootCurrent(metadata, nil); err != nil {
		return nil, err
	}
	p := &RestorePlan{}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.Close())
		}
	}()
	var release func()
	if permit != nil {
		release, err = permit(ctx)
		if err != nil {
			return nil, err
		}
		defer func() {
			if release != nil {
				release()
			}
		}()
	}
	base, err := content.Stat(".")
	if err != nil {
		return nil, err
	}
	p.content, err = newUpdateSet(content, base, ".tomori-pack-update-", permit)
	if err != nil {
		return nil, err
	}
	p.content.name, p.content.directory = name, filepath.Join(content.Name(), name)
	p.content.stage, p.content.stamp, err = openBackupRoot(content, name)
	if err != nil {
		return nil, err
	}
	if err := p.current(); err != nil {
		return nil, err
	}
	p.Info, err = backupSummary(ctx, p.content.stage, name)
	if err != nil {
		return nil, err
	}
	manifestInfo, err := p.content.stage.Lstat("manifest.json")
	if err != nil {
		return nil, err
	}
	p.manifestBytes[0] = manifestInfo.Size()
	records, err := readBackupRecords(ctx, p.content.stage)
	if err != nil {
		return nil, err
	}
	control, controlName, config, err := readUpdateControls(ctx, metadata, p.content.stage, true)
	if err != nil {
		return nil, err
	}
	if control == nil {
		return nil, errors.New(locales.BackupIsMissingInstanceConfiguration())
	}
	base, err = metadata.Stat(".")
	if err != nil {
		return nil, errors.Join(err, control.Close())
	}
	p.config, err = newUpdateSet(metadata, base, ".tomori-pack-config-", permit)
	if err != nil {
		return nil, errors.Join(err, control.Close())
	}
	p.config.name, p.config.directory, p.config.stage = controlName, filepath.Join(metadata.Name(), controlName), control
	p.config.stamp, err = control.Stat(".")
	if err != nil {
		return nil, err
	}
	manifestInfo, err = control.Lstat("manifest.json")
	if err != nil {
		return nil, err
	}
	p.manifestBytes[1] = manifestInfo.Size()
	foundOrigin := false
	for _, file := range config {
		if file.Path == OriginFile && file.Old && file.Next.Path != "" {
			foundOrigin = true
		}
	}
	if !foundOrigin {
		return nil, errors.New(locales.BackupIsMissingInstanceConfiguration())
	}
	for i, name := range []string{"next-origin", "previous-origin"} {
		r, _, openErr := openBackupRoot(p.content.stage, name)
		if openErr != nil {
			return nil, openErr
		}
		_, digest, readErr := ReadOriginSnapshot(ctx, r)
		stamp, statErr := r.Lstat(OriginFile)
		err = errors.Join(readErr, statErr, r.Close())
		if err != nil {
			return nil, err
		}
		p.origins[i] = updateCopy{info: stamp, hash: digest, size: stamp.Size()}
	}
	p.files, p.sources = make([]updateOperation, len(records)), make([]updateCopy, len(records))
	p.controls, p.configSources = make([]updateOperation, len(config)), make([]updateCopy, len(config))
	if release != nil {
		release()
		release = nil
	}
	err = updateWorkers(ctx, len(records)+len(config), permit, progress, func(ctx context.Context, i int) error {
		if i < len(records) {
			r := records[i]
			file, source, err := restoreOperation(ctx, p.content, i, r.Old, r.Target, r.BeforeSHA1, r.BeforeSize, r.BeforeMode, r.AfterMode, r.Entry.Next)
			p.files[i], p.sources[i] = file, source
			return err
		}
		i -= len(records)
		r := config[i]
		old := ""
		if r.Old {
			old = r.Path
		}
		file, source, err := restoreOperation(ctx, p.config, i, old, r.Path, r.BeforeSHA1, r.BeforeSize, r.BeforeMode, r.AfterMode, r.Next)
		p.controls[i], p.configSources[i] = file, source
		return err
	})
	if err != nil {
		return nil, err
	}
	return p, p.current()
}

func restoreOperation(ctx context.Context, set *updateSet, index int, old, target, hash string, size int64, mode uint32, afterMode *uint32, next InstalledFile) (updateOperation, updateCopy, error) {
	file := updateOperation{index: index, target: target}
	if old != "" {
		file.target = old
		file.next = InstalledFile{Path: old, SHA1: hash, Size: size}
	}
	var source updateCopy
	var err error
	if old != "" {
		source, err = updateHash(ctx, set.stage, backupFileName(index), nil)
		if err != nil {
			return file, source, err
		}
		if source.hash != hash || source.size != size || uint32(source.info.Mode().Perm()) != mode {
			return file, source, invalidBackup()
		}
	}
	if next.Path != "" {
		file.old = target
		file.previous, err = updateHash(ctx, set.root, target, nil)
		if err != nil {
			return file, source, err
		}
		if file.previous.hash != next.SHA1 || file.previous.size != next.Size || afterMode != nil && uint32(file.previous.info.Mode().Perm()) != *afterMode {
			return file, source, restoreConflict(target)
		}
	}
	if next.Path == "" || old != "" && old != target {
		name := old
		if name == "" {
			name = target
		}
		if err := restoreAbsent(set.root, name); err != nil {
			return file, source, err
		}
	}
	return file, source, nil
}

func restoreAbsent(root *os.Root, name string) error {
	blocked, missing, err := updateParents(root, name)
	if err != nil {
		return err
	}
	if blocked {
		return restoreConflict(name)
	}
	if missing {
		return nil
	}
	_, err = root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return errors.Join(err, restoreConflict(name))
}

func (p *RestorePlan) current() error {
	if p.content == nil || p.finished {
		return invalidBackup()
	}
	if err := p.content.current(); err != nil {
		return err
	}
	if err := backupMarker(p.content.stage, "committed"); err != nil {
		return err
	}
	if _, err := p.content.stage.Lstat("restored.json"); !errors.Is(err, os.ErrNotExist) {
		return errors.Join(err, errors.New(locales.ThisBackupHasAlreadyBeenRestored()))
	}
	if p.config != nil {
		if err := p.config.current(); err != nil {
			return err
		}
		return backupMarker(p.config.stage, "ready")
	}
	return nil
}

func (p *RestorePlan) AssessSpace(ctx context.Context) (storage.Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.assessSpace(ctx)
}

func (p *RestorePlan) assessSpace(ctx context.Context) (storage.Report, error) {
	if err := p.current(); err != nil {
		return storage.Report{}, err
	}
	var needs []storage.Requirement
	for i, item := range []struct {
		set   *updateSet
		files []updateOperation
	}{{p.content, p.files}, {p.config, p.controls}} {
		bytes := storage.Add(uint64(max(0, p.manifestBytes[i]))*2, 64<<10)
		if i == 0 {
			for _, origin := range p.origins {
				bytes = storage.Add(bytes, uint64(max(0, origin.size)))
			}
		}
		for _, file := range item.files {
			next := uint64(max(0, file.next.Size))
			bytes = storage.Add(bytes, storage.Add(next, next))
			bytes = storage.Add(bytes, uint64(max(0, file.previous.size)))
		}
		needs = append(needs, storage.Requirement{Directory: item.set.root.Name(), Bytes: bytes})
	}
	return storage.Assess(ctx, needs)
}

func (p *RestorePlan) validate(ctx context.Context, progress func(int, int)) error {
	if err := p.current(); err != nil {
		return err
	}
	return updateWorkers(ctx, len(p.files)+len(p.controls), p.content.permit, progress, func(ctx context.Context, i int) error {
		set, files := p.content, p.files
		if i >= len(files) {
			i -= len(files)
			set, files = p.config, p.controls
		}
		file := files[i]
		if file.old != "" {
			current, err := updateHash(ctx, set.root, file.old, file.previous.info)
			if err != nil {
				return err
			}
			if current.hash != file.previous.hash {
				return restoreConflict(file.old)
			}
		}
		if file.old == "" || file.old != file.target {
			if err := restoreAbsent(set.root, file.target); err != nil {
				return err
			}
		}
		return nil
	})
}

func (p *RestorePlan) Apply(ctx context.Context, progress func(int, int)) (backup string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	space, err := p.assessSpace(ctx)
	if err != nil {
		return "", err
	}
	if err := space.Check(); err != nil {
		return "", err
	}
	if err := p.validate(ctx, progress); err != nil {
		return "", err
	}
	set, err := newUpdateSet(p.content.root, p.content.base, ".tomori-pack-update-", p.content.permit)
	if err != nil {
		return "", err
	}
	t := &UpdateTransaction{updateSet: set}
	defer func() { err = errors.Join(err, t.Close()) }()
	t.files = slices.Clone(p.files)
	if err := t.makeStage(); err != nil {
		return "", err
	}
	for i, names := range [][2]string{{"next-origin", "previous-origin"}, {"previous-origin", "next-origin"}} {
		if err := t.stage.Mkdir(names[1], 0o700); err != nil {
			return "", err
		}
		if err := t.step(ctx, func() error {
			_, err := copyUpdateFile(ctx, p.content.stage, names[0]+"/"+OriginFile, t.stage, names[1]+"/"+OriginFile, p.origins[i].hash, p.origins[i].size, p.origins[i].info)
			return err
		}); err != nil {
			return "", err
		}
	}
	t.control, err = newUpdateSet(p.config.root, p.config.base, ".tomori-pack-config-", p.content.permit)
	if err != nil {
		return "", err
	}
	t.control.files = slices.Clone(p.controls)
	if err := t.control.makeStage(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(struct {
		Schema int    `json:"schema"`
		Root   string `json:"root"`
		Stage  string `json:"stage"`
	}{1, p.config.root.Name(), t.control.name})
	if err != nil {
		return "", err
	}
	err = updateWorkers(ctx, len(t.files)+len(t.control.files), p.content.permit, progress, func(ctx context.Context, i int) error {
		source, target, sources := p.content, t.updateSet, p.sources
		if i >= len(t.files) {
			i -= len(t.files)
			source, target, sources = p.config, t.control, p.configSources
		}
		file := &target.files[i]
		if file.next.Path == "" {
			return nil
		}
		if err := target.current(); err != nil {
			return err
		}
		var err error
		file.staged, err = copyUpdateFile(ctx, source.stage, backupFileName(file.index), target.stage, "next/"+strconv.Itoa(i), file.next.SHA1, file.next.Size, sources[i].info)
		return err
	})
	if err != nil {
		return "", err
	}
	if err := t.step(ctx, func() error {
		return t.writeManifest(ctx, func(i int) UpdateEntry {
			file := t.files[i]
			return UpdateEntry{Key: file.target, Path: file.target, Target: file.target, Next: file.next}
		})
	}); err != nil {
		return "", err
	}
	if err := t.control.step(ctx, func() error { return t.control.writeMetadataManifest(ctx, t.directory) }); err != nil {
		return "", err
	}
	if err := t.step(ctx, func() error { return fsutil.WriteRootAtomic(ctx, t.stage, "controls.json", raw) }); err != nil {
		return "", err
	}
	if err := p.validate(ctx, nil); err != nil {
		return "", err
	}
	if p.BeforeWrite != nil {
		if err := p.BeforeWrite(t.BackupPath()); err != nil {
			return "", err
		}
	}
	for _, set := range []*updateSet{t.updateSet, t.control} {
		for i := range slices.Backward(set.files) {
			if err := set.step(ctx, func() error { return set.applyFile(ctx, i) }); err != nil {
				return "", err
			}
		}
	}
	if err := t.Commit(ctx); err != nil {
		return "", err
	}
	backup, p.finished = t.BackupPath(), true
	raw, err = json.Marshal(struct {
		Backup string `json:"backup"`
	}{backup})
	if err == nil {
		err = p.content.step(context.WithoutCancel(ctx), func() error {
			return fsutil.WriteRootAtomic(context.WithoutCancel(ctx), p.content.stage, "restored.json", raw)
		})
	}
	return backup, err
}

func (p *RestorePlan) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	err := errors.Join(p.config.close(), p.content.close())
	p.config, p.content = nil, nil
	p.files, p.controls, p.sources, p.configSources = nil, nil, nil, nil
	return err
}
