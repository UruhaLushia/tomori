package modpack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
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

func (a *Archive) readPrism(ctx context.Context, name string) error {
	var m minecraft.PrismPack
	if err := a.readJSON(ctx, name, &m); err != nil {
		return err
	}
	a.Info = Info{Format: "Prism / MultiMC"}
	a.PrismComponents = slices.Clone(m.Components)
	componentVersions := make(map[string]string, len(m.Components))
	for _, c := range m.Components {
		version := c.Version
		if version == "" {
			version = c.CachedVersion
		}
		componentVersions[c.UID] = version
		if c.Disabled || strings.HasPrefix(c.UID, "org.lwjgl") || strings.HasPrefix(c.UID, "net.java") || c.UID == "net.minecraft.java" || c.UID == "net.fabricmc.intermediary" {
			continue
		}
		switch c.UID {
		case "net.minecraft", "net.minecraftforge", "net.neoforged", "net.fabricmc.fabric-loader", "org.quiltmc.quilt-loader", "com.mumfrey.liteloader":
			a.dependency(c.UID, version)
		}
	}
	patchEntries := make(map[string][]byte)
	for entry := range a.entries {
		if !strings.HasPrefix(entry, a.prefix+"patches/") || !strings.HasSuffix(entry, ".json") {
			continue
		}
		raw, err := a.read(ctx, entry)
		if err != nil {
			return err
		}
		uid := strings.TrimSuffix(path.Base(entry), ".json")
		patchEntries[uid] = raw
		if a.PrismPatches == nil {
			a.PrismPatches = make(map[string][]byte)
		}
		a.PrismPatches[path.Base(entry)] = raw
	}
	var patches []minecraft.VersionPatch
	for _, component := range m.Components {
		if component.Disabled {
			continue
		}
		raw, exists := patchEntries[component.UID]
		if !exists {
			continue
		}
		var patch minecraft.VersionPatch
		if err := json.Unmarshal(raw, &patch); err != nil {
			return fmt.Errorf(locales.Reading(), component.UID+".json", err)
		}
		patch.ID = component.UID
		if patch.Version == "" {
			patch.Version = componentVersions[component.UID]
		}
		patch.Priority = nil
		patches = append(patches, patch)
	}
	if len(patches) > 0 {
		a.Profile = &minecraft.VersionDetail{ID: a.Info.GameVersion, MinecraftVersion: a.Info.GameVersion, Patches: patches}
	}
	if _, exists := a.entries[a.prefix+"instance.cfg"]; exists {
		raw, err := a.read(ctx, a.prefix+"instance.cfg")
		if err != nil {
			return err
		}
		for line := range strings.SplitSeq(string(raw), "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			v = strings.TrimSpace(v)
			if unquoted, err := strconv.Unquote(v); err == nil {
				v = unquoted
			}
			switch k {
			case "name":
				a.Info.Name = v
			case "notes":
				a.Info.Description = strings.ReplaceAll(v, "\\n", "\n")
			}
		}
	}
	if _, found := a.entries[a.prefix+minecraft.PrismMetadataFile]; found {
		var meta minecraft.PrismMetadata
		if err := a.readJSON(ctx, a.prefix+minecraft.PrismMetadataFile, &meta); err != nil {
			return err
		}
		a.Info.Version = meta.PackVersion
	}
	a.overrides = []string{a.prefix + "minecraft/", a.prefix + ".minecraft/"}
	return nil
}

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

func (a *Archive) inferLoaders(p minecraft.VersionDetail) {
	a.Info.GameVersion = minecraft.ProfileGameVersion(&p, a.Info.GameVersion)
	for _, item := range minecraft.ProfileLoaders(&p, a.Info.GameVersion) {
		a.dependency(minecraft.ComponentLoader(item.ID), item.Version)
	}
}
