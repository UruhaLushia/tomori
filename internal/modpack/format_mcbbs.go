package modpack

import (
	"context"
	"fmt"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

func (a *Archive) readMCBBS(ctx context.Context, name string) error {
	var m struct {
		Name, Version, Author, Description string
		Addons                             []struct{ ID, Version string }
		Files                              []struct {
			Path, Hash        string
			ProjectID, FileID int64
			Required          *bool
		}
	}
	if err := a.readJSON(ctx, name, &m); err != nil {
		return err
	}
	a.Info = Info{Format: "MCBBS", Name: m.Name, Version: m.Version, Author: m.Author, Description: m.Description}
	for _, c := range m.Addons {
		a.dependency(c.ID, c.Version)
	}
	for _, f := range m.Files {
		if f.ProjectID > 0 && f.FileID > 0 {
			a.Info.Files = append(a.Info.Files, File{ProjectID: f.ProjectID, FileID: f.FileID, Optional: f.Required != nil && !*f.Required})
		} else if f.Path != "" {
			f.Path = strings.ReplaceAll(f.Path, "\\", "/")
			if err := validContentPath(f.Path); err != nil {
				return err
			}
			if f.Hash != "" && !validHash(f.Hash, 20) {
				return fmt.Errorf(locales.InvalidFileChecksumMetadata(), f.Path)
			}
			if a.bundled == nil {
				a.bundled = make(map[string]string)
			}
			if _, exists := a.bundled[f.Path]; exists {
				return fmt.Errorf(locales.DuplicateModpackFile(), f.Path)
			}
			a.bundled[f.Path] = f.Hash
		}
	}
	a.overrides = []string{a.prefix + "overrides/"}
	return nil
}
