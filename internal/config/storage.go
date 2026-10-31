package config

import (
	"encoding/json"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

func writeConfig(path string, format Format, cfg Config) error {
	if format == SQLite {
		return writeSQLite(path, cfg)
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(path, raw)
}
