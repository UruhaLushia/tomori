package modpack

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *Archive) readMinecraft(ctx context.Context) error {
	var profiles []string
	for name := range a.entries {
		parts := strings.Split(name, "/")
		if len(parts) >= 3 && parts[len(parts)-3] == "versions" && parts[len(parts)-1] == parts[len(parts)-2]+".json" {
			profiles = append(profiles, name)
		}
	}
	if len(profiles) != 1 {
		return errors.New(locales.NoModpackManifestFoundGameDirectoryArchivesMustContainASingleGameVersion())
	}
	profileName := profiles[0]
	var profile minecraft.VersionDetail
	if err := a.readJSON(ctx, profileName, &profile); err != nil {
		return err
	}
	if err := fsutil.ValidateID(profile.ID); err != nil {
		return err
	}
	a.Profile = &profile
	a.Info = Info{Format: "Minecraft", Name: profile.ID, GameVersion: profile.InheritsFrom}
	if a.Info.GameVersion == "" {
		a.Info.GameVersion = profile.ID
	}
	a.inferLoaders(profile)
	root := strings.TrimSuffix(profileName, "versions/"+profile.ID+"/"+path.Base(profileName))
	a.overrides = []string{root, root + "versions/" + profile.ID + "/"}
	return nil
}
