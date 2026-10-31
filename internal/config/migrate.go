package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const migrationFile = ".config-migration.json"

type migration struct {
	From  Format `json:"from"`
	To    Format `json:"to"`
	Stage string `json:"stage"`
}

// Migrate verifies the new store before removing the old one. The small journal
// allows startup to finish an interrupted rename without choosing between copies.
func (s *Store) Migrate(to Format, cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !to.valid() {
		return fmt.Errorf(locales.UnknownSettingsFormat(), to)
	}
	if to == SQLite && !SQLiteAvailable() {
		return errors.New(locales.SqLiteIsNotSupportedInThisBuild())
	}
	from := s.Format()
	if from == to {
		return nil
	}
	target := filepath.Join(s.dir, to.filename())
	if found, err := exists(target); err != nil {
		return err
	} else if found {
		return fmt.Errorf(locales.TargetConfigurationExists(), target)
	}
	f, err := os.CreateTemp(s.dir, ".config-migrate-*")
	if err != nil {
		return err
	}
	stage := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(stage)
		return err
	}
	defer func() { _ = os.Remove(stage) }()
	if err := writeConfig(stage, to, cfg); err != nil {
		return err
	}
	checked, err := readConfig(stage, to)
	if err != nil {
		return fmt.Errorf(locales.FailedToVerifyMigratedSettings(), err)
	}
	expected, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	actual, err := json.Marshal(checked)
	if err != nil {
		return err
	}
	// Compare serialized values: time.Time's monotonic clock is not persisted.
	if !bytes.Equal(actual, expected) {
		return errors.New(locales.MigratedSettingsVerificationMismatch())
	}
	m := migration{From: from, To: to, Stage: filepath.Base(stage)}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	journal := filepath.Join(s.dir, migrationFile)
	if err := fsutil.WriteAtomic(journal, raw); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		_ = os.Remove(journal)
		return err
	}
	if err := os.Remove(filepath.Join(s.dir, from.filename())); err != nil && !errors.Is(err, os.ErrNotExist) {
		// Keep the original active if the destination can be rolled back.
		if rollbackErr := os.Remove(target); rollbackErr == nil {
			_ = os.Remove(journal)
			return fmt.Errorf(locales.FailedToRemoveOriginalSettingsMigrationRolledBack(), err)
		}
		s.format.Store(uint32(to))
		return fmt.Errorf(locales.MigrationCleanupIncompleteRecoveryWillContinueAtRestart(), err)
	}
	s.format.Store(uint32(to))
	if err := os.Remove(journal); err != nil {
		return fmt.Errorf(locales.SettingsMigratedFailedToCleanMigrationRecord(), err)
	}
	return nil
}

func recoverMigration(dir string) error {
	journal := filepath.Join(dir, migrationFile)
	raw, err := os.ReadFile(journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var m migration
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	if !m.From.valid() || !m.To.valid() || m.From == m.To || filepath.Base(m.Stage) != m.Stage || !strings.HasPrefix(m.Stage, ".config-migrate-") {
		return errors.New(locales.InvalidSettingsMigrationRecord())
	}
	stage, target := filepath.Join(dir, m.Stage), filepath.Join(dir, m.To.filename())
	if found, err := exists(target); err != nil {
		return err
	} else if !found {
		if _, err := readConfig(stage, m.To); err != nil {
			// A failed rename rolled back before termination; the original is intact.
			if _, sourceErr := readConfig(filepath.Join(dir, m.From.filename()), m.From); sourceErr != nil {
				return errors.Join(err, sourceErr)
			}
			_ = os.Remove(stage)
			return os.Remove(journal)
		}
		if err := os.Rename(stage, target); err != nil {
			return err
		}
	}
	if _, err := readConfig(target, m.To); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, m.From.filename())); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(stage); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Remove(journal)
}
