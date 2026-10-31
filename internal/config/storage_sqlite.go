//go:build !no_sqlite

package config

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/UruhaLushia/tomori/locales"
)

func SQLiteAvailable() bool { return true }

func defaultFormat() Format { return SQLite }

type setting struct {
	Key   string `gorm:"primaryKey"`
	Value string `gorm:"not null"`
}

func (setting) TableName() string { return "settings" }

func openSQLite(path string, write bool) (*gorm.DB, error) {
	u := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}
	q := url.Values{"mode": {"rw"}, "_pragma": {"busy_timeout(3000)"}}
	if write {
		q.Set("mode", "rwc")
		q.Add("_pragma", "journal_mode(DELETE)")
		q.Add("_pragma", "synchronous(FULL)")
	}
	u.RawQuery = q.Encode()
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	return db, nil
}

func writeSQLite(path string, cfg Config) error {
	db, err := openSQLite(path, true)
	if err != nil {
		return err
	}
	conn, err := db.DB()
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db = db.WithContext(ctx)
	if err := db.AutoMigrate(&setting{}); err != nil {
		return err
	}
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
	return db.Transaction(func(tx *gorm.DB) error {
		var previous []setting
		if err := tx.Find(&previous).Error; err != nil {
			return err
		}
		if len(previous) > 0 {
			if err := tx.Delete(&previous).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&groups).Error
	})
}

func readSQLite(path string) (Config, error) {
	cfg := defaults()
	db, err := openSQLite(path, false)
	if err != nil {
		return cfg, err
	}
	conn, err := db.DB()
	if err != nil {
		return cfg, err
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var rows []setting
	if err := db.WithContext(ctx).Find(&rows).Error; err != nil {
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
