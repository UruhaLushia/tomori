package modpack

import "github.com/UruhaLushia/tomori/internal/minecraft"

func (a *Archive) inferLoaders(p minecraft.VersionDetail) {
	a.Info.GameVersion = minecraft.ProfileGameVersion(&p, a.Info.GameVersion)
	for _, item := range minecraft.ProfileLoaders(&p, a.Info.GameVersion) {
		a.dependency(minecraft.ComponentLoader(item.ID), item.Version)
	}
}
