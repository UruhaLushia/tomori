package modpack

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/UruhaLushia/tomori/locales"
)

func (a *Archive) readModrinth(ctx context.Context, name string) error {
	var m struct {
		FormatVersion                  int
		Game, Name, VersionID, Summary string
		Dependencies                   map[string]string
		Files                          []struct {
			Path      string
			Hashes    map[string]string
			Env       map[string]string
			Downloads []string
			FileSize  int64
		}
	}
	if err := a.readJSON(ctx, name, &m); err != nil {
		return err
	}
	if m.FormatVersion != 1 || m.Game != "minecraft" {
		return errors.New(locales.UnsupportedModrinthModpackFormatVersion())
	}
	a.Info = Info{Format: "Modrinth", Name: m.Name, Version: m.VersionID, Description: m.Summary}
	keys := make([]string, 0, len(m.Dependencies))
	for id := range m.Dependencies {
		keys = append(keys, id)
	}
	slices.Sort(keys)
	for _, id := range keys {
		a.dependency(id, m.Dependencies[id])
	}
	for _, f := range m.Files {
		if f.Env["client"] == "unsupported" {
			continue
		}
		if f.Env["client"] != "" && !slices.Contains([]string{"required", "optional"}, f.Env["client"]) {
			return fmt.Errorf(locales.UnknownClientFileType(), f.Path)
		}
		a.Info.Files = append(a.Info.Files, File{Path: f.Path, SHA1: f.Hashes["sha1"], SHA512: f.Hashes["sha512"], Size: f.FileSize, URLs: f.Downloads, Optional: f.Env["client"] == "optional"})
	}
	a.overrides = []string{a.prefix + "overrides/", a.prefix + "client-overrides/"}
	return nil
}
