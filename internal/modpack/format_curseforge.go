package modpack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *Archive) readCurseForge(ctx context.Context, name string) error {
	var m struct {
		ManifestType                     string
		ManifestVersion                  int
		Name, Version, Author, Overrides string
		Minecraft                        struct {
			Version    string
			ModLoaders []struct{ ID string }
		}
		Files []struct {
			ProjectID, FileID int64
			Required          *bool
		}
	}
	if err := a.readJSON(ctx, name, &m); err != nil {
		return err
	}
	if m.ManifestType != "minecraftModpack" || m.ManifestVersion != 1 {
		return errors.New(locales.UnsupportedCurseForgeModpackFormatVersion())
	}
	a.Info = Info{Format: "CurseForge", Name: m.Name, Version: m.Version, Author: m.Author, GameVersion: m.Minecraft.Version}
	for _, l := range m.Minecraft.ModLoaders {
		id, version, ok := strings.Cut(l.ID, "-")
		if !ok {
			return fmt.Errorf(locales.InvalidLoader(), l.ID)
		}
		a.dependency(id, version)
	}
	for _, f := range m.Files {
		if f.ProjectID <= 0 || f.FileID <= 0 {
			return errors.New(locales.InvalidCurseForgeFileId())
		}
		a.Info.Files = append(a.Info.Files, File{ProjectID: f.ProjectID, FileID: f.FileID, Optional: f.Required != nil && !*f.Required})
	}
	if m.Overrides == "" {
		m.Overrides = "overrides"
	}
	if m.Overrides == "." || m.Overrides == "./" {
		a.overrides = []string{a.prefix}
	} else {
		if _, err := fsutil.SafePath(".", m.Overrides); err != nil {
			return err
		}
		a.overrides = []string{a.prefix + strings.Trim(m.Overrides, "/") + "/"}
	}
	return nil
}
