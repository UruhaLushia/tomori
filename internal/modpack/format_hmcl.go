package modpack

import (
	"context"

	"github.com/UruhaLushia/tomori/internal/minecraft"
)

func (a *Archive) readHMCL(ctx context.Context, name string) error {
	var m struct{ Name, Version, Author, Description, GameVersion string }
	if err := a.readJSON(ctx, name, &m); err != nil {
		return err
	}
	a.Info = Info{Format: "HMCL", Name: m.Name, Version: m.Version, Author: m.Author, Description: m.Description, GameVersion: m.GameVersion}
	a.overrides = []string{a.prefix + "minecraft/"}
	// HMCL exports may include a complete launch profile named pack.json.
	if _, ok := a.entries[a.prefix+"minecraft/pack.json"]; ok {
		var profile minecraft.VersionDetail
		if err := a.readJSON(ctx, a.prefix+"minecraft/pack.json", &profile); err != nil {
			return err
		}
		if profile.MainClass != "" {
			a.Profile = &profile
			a.inferLoaders(profile)
		}
	}
	return nil
}
