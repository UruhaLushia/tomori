package config

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/UruhaLushia/tomori/internal/sqlite"
	"github.com/UruhaLushia/tomori/locales"
)

type setting struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

func writeSQLite(path string, cfg Config) error {
	db, err := sqlite.Open(path, true, &setting{})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	groups := make([]setting, 0, len(values))
	for key, value := range values {
		groups = append(groups, setting{Key: key, Value: string(value)})
	}
	return db.Replace(ctx, &[]setting{}, &groups)
}

func readSQLite(path string) (Config, error) {
	cfg := defaults()
	db, err := sqlite.Open(path, false, &setting{})
	if err != nil {
		return cfg, err
	}
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rows []setting
	if err := db.Read(ctx, &rows, ""); err != nil {
		return cfg, err
	}
	groups := map[string]json.RawMessage{}
	for _, row := range rows {
		groups[row.Key] = json.RawMessage(row.Value)
	}
	if len(groups) == 0 {
		return cfg, errors.New(locales.SettingsDatabaseIsEmpty())
	}
	raw, err := json.Marshal(groups)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(raw, &cfg)
	return cfg, err
}
