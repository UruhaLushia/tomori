package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/UruhaLushia/tomori/internal/logstore"
	"github.com/UruhaLushia/tomori/locales"
)

// Format is inferred from the single active configuration file.
type Format uint32

const (
	SQLite Format = iota
	JSON
)

func (f Format) String() string {
	if f == JSON {
		return "JSON"
	}
	return "SQLite"
}
func (f Format) filename() string {
	if f == JSON {
		return "config.json"
	}
	return "config.db"
}
func (f Format) valid() bool { return f == SQLite || f == JSON }

// Store keeps settings and framework-managed window state in one active file.
type Store struct {
	Config Config
	dir    string
	format atomic.Uint32
	mu     sync.Mutex
	logs   *logstore.Store
}

func Open() (*Store, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, fmt.Errorf(locales.FailedToLocateSettingsDirectory(), err)
		}
		dir = filepath.Join(home, ".config")
	}
	dir = filepath.Join(dir, "tomori")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := recoverMigration(dir); err != nil {
		return nil, fmt.Errorf(locales.FailedToRecoverSettingsMigration(), err)
	}
	s := &Store{Config: defaults(), dir: dir}
	s.format.Store(uint32(defaultFormat()))
	dbExists, err := exists(filepath.Join(dir, SQLite.filename()))
	if err != nil {
		return nil, err
	}
	jsonExists, err := exists(filepath.Join(dir, JSON.filename()))
	if err != nil {
		return nil, err
	}
	if dbExists && !SQLiteAvailable() {
		return nil, errors.New(locales.SqLiteIsNotSupportedInThisBuildConfigDbWasNotModifiedOpenItWithABuildThatIncludesSqLite())
	}
	if dbExists && jsonExists {
		return nil, errors.New(locales.BothConfigDbAndConfigJsonExistSettingsSourceIsAmbiguous())
	}
	if jsonExists {
		s.format.Store(uint32(JSON))
	}
	if !dbExists && !jsonExists {
		if err := s.Save(s.Config); err != nil {
			return nil, err
		}
		return s, nil
	}
	cfg, err := readConfig(s.Path(), s.Format())
	if err != nil {
		return nil, fmt.Errorf(locales.ConfigurationReadFailed(), s.Path(), err)
	}
	legacyFont := cfg.Appearance.FontSize != 0
	cfg.normalize()
	s.Config = cfg
	if legacyFont {
		if err := s.Save(cfg); err != nil {
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) Format() Format     { return Format(s.format.Load()) }
func (s *Store) Path() string       { return filepath.Join(s.dir, s.Format().filename()) }
func (s *Store) Dir() string        { return s.dir }
func (s *Store) RuntimeDir() string { return filepath.Join(s.dir, "runtime") }
func (s *Store) LogDir() string     { return filepath.Join(s.dir, "logs") }

func (s *Store) LogStore() (*logstore.Store, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.logs == nil {
		limit := s.Config.Logging.MaxMiB
		if limit <= 0 {
			limit = 256
		}
		logs, err := logstore.New(s.LogDir(), int64(max(16, min(limit, 4096)))<<20)
		if err != nil {
			return nil, err
		}
		s.logs = logs
	}
	return s.logs, nil
}

// Save accepts a value snapshot so background persistence never reads UI state.
func (s *Store) Save(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeConfig(s.Path(), s.Format(), cfg)
}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func readConfig(path string, format Format) (Config, error) {
	if format == SQLite {
		return readSQLite(path)
	}
	cfg := defaults()
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(raw, &cfg)
	return cfg, err
}
