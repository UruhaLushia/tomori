package launcher

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/storage"
)

type PackRestore struct {
	Plan interface {
		AssessSpace(context.Context) (storage.Report, error)
		Apply(context.Context, func(int, int)) (string, error)
		Close() error
	}
	Info     modpack.BackupInfo
	instance minecraft.Instance
	content  string
	recovery bool
	finish   func() error
}

func (s *Service) packRestoreRoots(ctx context.Context, instance minecraft.Instance) (content, metadata *os.Root, err error) {
	metadata, err = minecraft.OpenInstanceRoot(ctx, instance)
	if err != nil {
		return nil, nil, err
	}
	name := instance.ContentDirectory(s.cfg.Game.Isolate)
	canonical, err := filepath.EvalSymlinks(name)
	if err == nil && canonical != filepath.Clean(name) {
		err = packUpdateChanged()
	}
	if err == nil {
		content, err = os.OpenRoot(name)
	}
	if err != nil {
		return nil, nil, errors.Join(err, metadata.Close())
	}
	return content, metadata, nil
}

func (s *Service) PackBackups(ctx context.Context, instance minecraft.Instance) (_ []modpack.BackupInfo, err error) {
	s = s.forInstance(instance)
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	content, metadata, err := s.packRestoreRoots(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, content.Close(), metadata.Close()) }()
	backups, err := modpack.ListBackups(ctx, content, true)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(backups, func(a, b modpack.BackupInfo) int {
		if a.Interrupted != b.Interrupted {
			if a.Interrupted {
				return -1
			}
			return 1
		}
		return b.CreatedAt.Compare(a.CreatedAt)
	})
	return backups, nil
}

func (s *Service) PreparePackRestore(ctx context.Context, instance minecraft.Instance, info modpack.BackupInfo, progress func(minecraft.Progress)) (prepared *PackRestore, err error) {
	s = s.forInstance(instance)
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return nil, err
	}
	defer release()
	content, metadata, err := s.packRestoreRoots(ctx, instance)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, content.Close(), metadata.Close())
		if err != nil && prepared != nil {
			err = errors.Join(err, prepared.Plan.Close())
			prepared = nil
		}
	}()
	report := func(done, total int) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-restore-check", FilesDone: done, Files: total})
		}
	}
	if info.Interrupted {
		plan, err := modpack.PrepareRecovery(ctx, content, metadata, info.Name, s.client.FilePermit, report)
		if err != nil {
			return nil, err
		}
		finish := func() error {
			_, _, err := s.PendingPackRecovery(context.WithoutCancel(ctx), info.Name+".json")
			return err
		}
		return &PackRestore{Plan: plan, Info: plan.Info, instance: instance, content: content.Name(), finish: finish}, nil
	}
	plan, err := modpack.PrepareRestore(ctx, content, metadata, info.Name, s.client.FilePermit, report)
	if err != nil {
		return nil, err
	}
	before, finish := s.packRecoveryTracker(ctx, instance, content.Name())
	plan.BeforeWrite = before
	return &PackRestore{Plan: plan, Info: plan.Info, instance: instance, content: content.Name(), finish: finish}, nil
}

func (s *Service) ApplyPackRestore(ctx context.Context, prepared *PackRestore, progress func(minecraft.Progress)) (backup string, err error) {
	if prepared == nil || prepared.Plan == nil {
		return "", packUpdateChanged()
	}
	s = s.forInstance(prepared.instance)
	if !prepared.recovery && filepath.Clean(prepared.content) != filepath.Clean(prepared.instance.ContentDirectory(s.cfg.Game.Isolate)) {
		return "", packUpdateChanged()
	}
	release, err := s.lockRoot(ctx, prepared.instance.GameDir, progress)
	if err != nil {
		return "", err
	}
	defer release()
	if prepared.finish != nil {
		defer func() { err = errors.Join(err, prepared.finish()) }()
	}
	report := func(done, total int) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-restore", FilesDone: done, Files: total})
		}
	}
	return prepared.Plan.Apply(ctx, report)
}
