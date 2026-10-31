//go:build no_sqlite

package config

import "errors"

func SQLiteAvailable() bool { return false }

func defaultFormat() Format { return JSON }

func writeSQLite(string, Config) error {
	return errors.New("SQLite support is not available in this build")
}

func readSQLite(string) (Config, error) {
	return defaults(), errors.New("SQLite support is not available in this build")
}
