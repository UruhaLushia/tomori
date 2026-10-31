package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type PackRecovery struct {
	Instance minecraft.Instance
	Content  string
	Info     modpack.BackupInfo
}

type packRecoveryRecord struct {
	Schema                                          int `json:"schema"`
	ID, Name, InstanceDir, GameDir, Content, Backup string
	Format                                          minecraft.InstanceFormat
}

func (r packRecoveryRecord) instance() minecraft.Instance {
	return minecraft.Instance{VersionSummary: minecraft.VersionSummary{ID: r.ID}, Name: r.Name, Format: r.Format, InstanceDir: r.InstanceDir, GameDir: r.GameDir}
}

func invalidRecoveryRecord() error { return errors.New(locales.InvalidInterruptedUpdateRecord()) }

func (s *Service) packRecoveryRoot(create bool) (*os.Root, error) {
	directory := filepath.Join(s.store.Dir(), "pack-recovery")
	if create {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(directory)
}

func readPackRecoveryRecord(root *os.Root, name string) (packRecoveryRecord, error) {
	var record packRecoveryRecord
	if fsutil.ValidateID(name) != nil || !strings.HasPrefix(name, ".tomori-pack-update-") || !strings.HasSuffix(name, ".json") {
		return record, invalidRecoveryRecord()
	}
	raw, err := fsutil.ReadRootFile(root, name, 64<<10)
	if err != nil {
		return record, err
	}
	if err := json.Unmarshal(raw, &record); err != nil {
		return record, err
	}
	if record.Schema != 1 || record.Backup+".json" != name || record.Name == "" || !record.instance().CanLaunch() {
		return record, invalidRecoveryRecord()
	}
	for _, directory := range []string{record.InstanceDir, record.GameDir, record.Content} {
		if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || filepath.Dir(directory) == directory {
			return record, invalidRecoveryRecord()
		}
		if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == directory {
			return record, invalidRecoveryRecord()
		}
	}
	switch record.Format {
	case minecraft.InstanceManaged:
		if record.GameDir != filepath.Join(record.InstanceDir, "minecraft") || record.Content != record.GameDir {
			return record, invalidRecoveryRecord()
		}
	case minecraft.InstancePrism:
		base := filepath.Base(record.GameDir)
		if filepath.Dir(record.GameDir) != record.InstanceDir || base != "minecraft" && base != ".minecraft" || record.Content != record.GameDir {
			return record, invalidRecoveryRecord()
		}
	case minecraft.InstanceMinecraft:
		if fsutil.ValidateID(record.ID) != nil || record.InstanceDir != filepath.Join(record.GameDir, "versions", record.ID) || record.Content != record.GameDir && record.Content != record.InstanceDir {
			return record, invalidRecoveryRecord()
		}
	}
	return record, nil
}

func openPackRecoveryDirectory(ctx context.Context, name string) (*os.Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(name)
	if err != nil {
		return nil, err
	}
	if canonical != name {
		return nil, packUpdateChanged()
	}
	return os.OpenRoot(name)
}

func (s *Service) packRecoveryTracker(ctx context.Context, instance minecraft.Instance, content string) (func(string) error, func() error) {
	name := ""
	before := func(backup string) error {
		if filepath.Dir(backup) != filepath.Clean(content) {
			return invalidRecoveryRecord()
		}
		record := packRecoveryRecord{Schema: 1, ID: instance.ID, Name: instance.Name, Format: instance.Format, InstanceDir: instance.InstanceDir, GameDir: instance.GameDir, Content: content, Backup: filepath.Base(backup)}
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		root, err := s.packRecoveryRoot(true)
		if err != nil {
			return err
		}
		defer root.Close()
		name = record.Backup + ".json"
		return fsutil.WriteRootAtomic(ctx, root, name, raw)
	}
	finish := func() error {
		if name == "" {
			return nil
		}
		_, _, err := s.PendingPackRecovery(context.WithoutCancel(ctx), name)
		return err
	}
	return before, finish
}

func (s *Service) readPendingPackRecovery(ctx context.Context, root *os.Root, name string) (PackRecovery, bool, error) {
	record, err := readPackRecoveryRecord(root, name)
	if err != nil {
		return PackRecovery{}, false, err
	}
	item := PackRecovery{Instance: record.instance(), Content: record.Content, Info: modpack.BackupInfo{Name: record.Backup, Interrupted: true}}
	content, err := openPackRecoveryDirectory(ctx, record.Content)
	if err != nil {
		item.Info.Issue = err.Error()
		return item, true, nil
	}
	defer content.Close()
	info, pending, err := modpack.ReadInterruptedUpdate(ctx, content, record.Backup)
	if err != nil {
		item.Info.Issue = err.Error()
		return item, true, nil
	}
	if !pending {
		err := root.Remove(name)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		return item, false, err
	}
	item.Info = info
	return item, true, nil
}

func (s *Service) PendingPackRecovery(ctx context.Context, name string) (PackRecovery, bool, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return PackRecovery{}, false, err
	}
	defer release()
	root, err := s.packRecoveryRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return PackRecovery{}, false, nil
	}
	if err != nil {
		return PackRecovery{}, false, err
	}
	defer root.Close()
	item, pending, err := s.readPendingPackRecovery(ctx, root, name)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return item, pending, err
}

func (s *Service) PendingPackRecoveries(ctx context.Context) ([]PackRecovery, error) {
	root, err := s.packRecoveryRoot(false)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	var items []PackRecovery
	var problems []error
	count := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := directory.ReadDir(64)
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".tomori-pack-update-") || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			count++
			if count > 512 {
				return items, errors.New(locales.TooManyInterruptedUpdatesOrganizeTheRecoveryFolderFirst())
			}
			release, err := s.client.FilePermit(ctx)
			if err != nil {
				return nil, err
			}
			item, pending, err := s.readPendingPackRecovery(ctx, root, entry.Name())
			release()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				problems = append(problems, err)
			} else if pending {
				items = append(items, item)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return items, readErr
		}
	}
	slices.SortStableFunc(items, func(a, b PackRecovery) int { return b.Info.CreatedAt.Compare(a.Info.CreatedAt) })
	return items, errors.Join(problems...)
}

func (s *Service) PreparePendingPackRestore(ctx context.Context, recovery PackRecovery, progress func(minecraft.Progress)) (_ *PackRestore, err error) {
	item, pending, err := s.PendingPackRecovery(ctx, recovery.Info.Name+".json")
	if err != nil {
		return nil, err
	}
	if !pending || item.Info.Issue != "" {
		return nil, packUpdateChanged()
	}
	release, err := s.lockRoot(ctx, item.Instance.GameDir, progress)
	if err != nil {
		return nil, err
	}
	defer release()
	metadata, err := openPackRecoveryDirectory(ctx, item.Instance.InstanceDir)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, metadata.Close()) }()
	content, err := openPackRecoveryDirectory(ctx, item.Content)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, content.Close()) }()
	plan, err := modpack.PrepareRecovery(ctx, content, metadata, item.Info.Name, s.client.FilePermit, func(done, total int) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-restore-check", FilesDone: done, Files: total})
		}
	})
	if err != nil {
		return nil, err
	}
	finish := func() error {
		_, _, err := s.PendingPackRecovery(context.WithoutCancel(ctx), item.Info.Name+".json")
		return err
	}
	return &PackRestore{Plan: plan, Info: plan.Info, instance: item.Instance, content: item.Content, recovery: true, finish: finish}, nil
}
