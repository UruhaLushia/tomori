package modpack

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

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
		if strings.HasPrefix(entry, a.prefix+"libraries/") {
			relative := strings.TrimPrefix(entry, a.prefix+"libraries/")
			if relative != "" && !a.entries[entry].FileInfo().IsDir() {
				if a.PrismLibraries == nil {
					a.PrismLibraries = make(map[string]*zip.File)
				}
				a.PrismLibraries[relative] = a.entries[entry]
			}
		}
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
	mods := make(map[string]struct{})
	for entry := range a.entries {
		for _, root := range []string{"minecraft/mods/", ".minecraft/mods/"} {
			prefix := a.prefix + root
			if !strings.HasPrefix(entry, prefix) {
				continue
			}
			name := strings.TrimPrefix(entry, prefix)
			if strings.Contains(name, "/") {
				continue
			}
			switch strings.ToLower(path.Ext(name)) {
			case ".jar", ".zip", ".litemod":
				mods[name] = struct{}{}
			}
		}
	}
	a.Info.ModCount = len(mods)
	return nil
}
