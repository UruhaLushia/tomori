package minecraft

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type InstalledComponent struct {
	ID, Name, Version string
	Disabled          bool
}

type InstanceComponents struct {
	Game, Loader, LoaderVersion, Reason string
	Items                               []InstalledComponent
	Fingerprint                         [32]byte
	ProfileFingerprint                  [32]byte
}

func LoaderUID(id string) string {
	switch id {
	case "fabric":
		return "net.fabricmc.fabric-loader"
	case "quilt":
		return "org.quiltmc.quilt-loader"
	case "forge":
		return "net.minecraftforge"
	case "neoforge":
		return "net.neoforged"
	}
	return ""
}

func ComponentLoader(uid string) string {
	for _, id := range []string{"fabric", "quilt", "forge", "neoforge"} {
		if LoaderUID(id) == uid {
			return id
		}
	}
	return ""
}

func InspectComponents(ctx context.Context, instance Instance) (InstanceComponents, error) {
	return inspectComponents(ctx, instance, true)
}

func InspectExportComponents(ctx context.Context, instance Instance) (InstanceComponents, error) {
	return inspectComponents(ctx, instance, false)
}

func inspectComponents(ctx context.Context, instance Instance, protect bool) (InstanceComponents, error) {
	var result InstanceComponents
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return result, err
	}
	defer func() { _ = root.Close() }()
	if instance.Format == InstancePrism {
		raw, err := fsutil.ReadRootFile(root, "mmc-pack.json", 4<<20)
		if err != nil {
			return result, err
		}
		result.Fingerprint = sha256.Sum256(raw)
		var pack PrismPack
		if err := json.Unmarshal(raw, &pack); err != nil {
			return result, err
		}
		if err := validatePrismPack(pack); err != nil {
			return result, err
		}
		for _, item := range pack.Components {
			name := item.UID
			id := ComponentLoader(item.UID)
			if id != "" {
				name = loaderName(id)
				if !item.Disabled {
					if result.Loader != "" {
						result.Reason = locales.InstancesWithMultipleLoadersCannotBeModifiedDirectly()
					}
					result.Loader, result.LoaderVersion = id, item.Version
				}
			} else {
				switch item.UID {
				case "net.minecraft":
					name = "Minecraft"
					if !item.Disabled {
						result.Game = item.Version
					}
				case "org.lwjgl", "org.lwjgl3":
					name = "LWJGL"
				case "net.minecraft.java", "net.java":
					name = "Java"
				case "net.fabricmc.intermediary":
					name = "Intermediary"
				default:
					if !item.Disabled {
						result.Reason = fmt.Sprintf(locales.ComponentMustBeManagedInTheOriginalLauncher(), item.UID)
					}
				}
			}
			result.Items = append(result.Items, InstalledComponent{ID: item.UID, Name: name, Version: item.Version, Disabled: item.Disabled})
		}
		if result.Game == "" {
			return result, errors.New(locales.InstanceHasNoMinecraftVersion())
		}
		if patch, err := customPrismPatch(root); err != nil {
			return result, err
		} else if patch != "" {
			result.Reason = fmt.Sprintf(locales.CustomPatchMustBeManagedInTheOriginalLauncher(), patch)
		}
		return result, ctx.Err()
	}
	profile, err := componentProfileRoot(root, instance)
	if err != nil {
		return result, err
	}
	defer func() { _ = profile.Close() }()
	raw, err := fsutil.ReadRootFile(profile, instance.ID+".json", 16<<20)
	if err != nil {
		return result, err
	}
	result.ProfileFingerprint = sha256.Sum256(raw)
	detail, err := ReadLocalVersion(instance.GameDir, instance.ID)
	if err != nil {
		return result, err
	}
	materialized, err := json.Marshal(detail)
	if err != nil {
		return result, err
	}
	hash := sha256.New()
	_, _ = hash.Write(raw)
	_, _ = hash.Write(materialized)
	copy(result.Fingerprint[:], hash.Sum(nil))
	result.Game = ProfileGameVersion(detail, instance.GameVersion)
	result.Items = []InstalledComponent{{ID: "net.minecraft", Name: "Minecraft", Version: result.Game}}
	loaders := ProfileLoaders(detail, result.Game)
	for _, item := range loaders {
		result.Items = append(result.Items, item)
		result.Loader, result.LoaderVersion = ComponentLoader(item.ID), item.Version
	}
	if len(loaders) > 1 {
		result.Reason = locales.InstancesWithMultipleLoadersCannotBeModifiedDirectly()
	}
	switch detail.MainClass {
	case "net.minecraft.client.main.Main", "net.minecraft.launchwrapper.Launch", "cpw.mods.modlauncher.Launcher", "cpw.mods.bootstraplauncher.BootstrapLauncher", "net.fabricmc.loader.impl.launch.knot.KnotClient", "net.fabricmc.loader.launch.knot.KnotClient", "org.quiltmc.loader.impl.launch.knot.KnotClient":
	default:
		result.Reason = fmt.Sprintf(locales.CustomMainClassMustBeManagedInTheOriginalLauncher(), detail.MainClass)
	}
	for _, library := range detail.Libraries {
		if strings.HasPrefix(strings.ToLower(library.Name), "optifine:") || strings.HasPrefix(library.Name, "com.mumfrey:liteloader:") {
			result.Reason = fmt.Sprintf(locales.ComponentMustBeManagedInTheOriginalLauncher(), library.Name)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return result, err
	}
	for _, key := range []string{"patches", "jarMods", "+libraries", "+tweakers", "tweakers"} {
		if _, exists := fields[key]; exists {
			result.Reason = fmt.Sprintf(locales.CustomPatchMustBeManagedInTheOriginalLauncher(), key)
		}
	}
	if protect && instance.Format == InstanceMinecraft && result.Reason == "" {
		if _, err := InstanceTrashPath(ctx, instance); err != nil {
			result.Reason = err.Error()
		}
	}
	return result, ctx.Err()
}

func loaderName(id string) string {
	switch id {
	case "fabric":
		return "Fabric"
	case "quilt":
		return "Quilt"
	case "forge":
		return "Forge"
	case "neoforge":
		return "NeoForge"
	}
	return id
}

func customPrismPatch(root *os.Root) (string, error) {
	patches, err := root.OpenRoot("patches")
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer patches.Close()
	dir, err := patches.Open(".")
	if err != nil {
		return "", err
	}
	defer func() { _ = dir.Close() }()
	for {
		entries, err := dir.ReadDir(128)
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				return entry.Name(), nil
			}
		}
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
}

func componentProfileRoot(root *os.Root, instance Instance) (*os.Root, error) {
	if instance.Format == InstanceManaged {
		return root.OpenRoot(filepath.Join("minecraft", "versions", instance.ID))
	}
	return root.OpenRoot(".")
}

type ComponentFile struct {
	Path, ExpectedSHA1 string
	Raw                []byte
}

func PrepareComponentFile(ctx context.Context, instance Instance, expected [32]byte, detail *VersionDetail, prismRaw []byte) (ComponentFile, error) {
	var result ComponentFile
	err := componentConfiguration(ctx, instance, expected, detail, prismRaw, func(root *os.Root, name string, raw, previous []byte) error {
		relative, err := filepath.Rel(instance.InstanceDir, filepath.Join(root.Name(), name))
		if err != nil || !filepath.IsLocal(relative) {
			return errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
		}
		hash := sha1.Sum(previous)
		result = ComponentFile{Path: filepath.ToSlash(relative), Raw: raw, ExpectedSHA1: hex.EncodeToString(hash[:])}
		return nil
	})
	return result, err
}

func CommitComponents(ctx context.Context, instance Instance, expected [32]byte, detail *VersionDetail, prismRaw []byte) error {
	return componentConfiguration(ctx, instance, expected, detail, prismRaw, func(root *os.Root, name string, raw, previous []byte) error {
		return fsutil.WriteRootAtomic(ctx, root, name, raw)
	})
}

func componentConfiguration(ctx context.Context, instance Instance, expected [32]byte, detail *VersionDetail, prismRaw []byte, write func(*os.Root, string, []byte, []byte) error) error {
	current, err := InspectComponents(ctx, instance)
	if err != nil {
		return err
	}
	if current.Fingerprint != expected {
		return errors.New(locales.InstanceComponentsChangedReopenThisDialogToEditThem())
	}
	if current.Reason != "" {
		return errors.New(current.Reason)
	}
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if instance.Format == InstancePrism {
		previous, err := fsutil.ReadRootFile(root, "mmc-pack.json", 4<<20)
		if err != nil {
			return err
		}
		if sha256.Sum256(previous) != expected {
			return errors.New(locales.InstanceComponentsChangedReopenThisDialogToEditThem())
		}
		var pack PrismPack
		if err := json.Unmarshal(prismRaw, &pack); err != nil {
			return err
		}
		if err := validatePrismPack(pack); err != nil {
			return err
		}
		if !slices.ContainsFunc(pack.Components, func(item PrismComponent) bool {
			return item.UID == "net.minecraft" && !item.Disabled
		}) {
			return errors.New(locales.InstanceHasNoMinecraftVersion())
		}
		return write(root, "mmc-pack.json", prismRaw, previous)
	}
	if err := detail.Validate(); err != nil {
		return err
	}
	if detail.ID != instance.ID {
		return errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
	}
	profile, err := componentProfileRoot(root, instance)
	if err != nil {
		return err
	}
	defer func() { _ = profile.Close() }()
	file := instance.ID + ".json"
	previous, err := fsutil.ReadRootFile(profile, file, 16<<20)
	if err != nil {
		return err
	}
	if sha256.Sum256(previous) != current.ProfileFingerprint {
		return errors.New(locales.InstanceComponentsChangedReopenThisDialogToEditThem())
	}
	var fields, replacement map[string]json.RawMessage
	if err := json.Unmarshal(previous, &fields); err != nil {
		return err
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &replacement); err != nil {
		return err
	}
	delete(fields, "inheritsFrom")
	maps.Copy(fields, replacement)
	raw, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	return write(profile, file, raw, previous)
}

func UpdatedPrismComponents(ctx context.Context, instance Instance, game, loaderID, version string) (PrismPack, []byte, error) {
	if err := fsutil.ValidateID(game); err != nil {
		return PrismPack{}, nil, err
	}
	if loaderID != "" {
		if LoaderUID(loaderID) == "" {
			return PrismPack{}, nil, fmt.Errorf(locales.UnsupportedLoader2(), loaderID)
		}
		if err := fsutil.ValidateID(version); err != nil {
			return PrismPack{}, nil, err
		}
	}
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return PrismPack{}, nil, err
	}
	defer func() { _ = root.Close() }()
	raw, err := fsutil.ReadRootFile(root, "mmc-pack.json", 4<<20)
	if err != nil {
		return PrismPack{}, nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return PrismPack{}, nil, err
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(fields["components"], &items); err != nil {
		return PrismPack{}, nil, err
	}
	var previousGame string
	for _, item := range items {
		var uid string
		_ = json.Unmarshal(item["uid"], &uid)
		if uid == "net.minecraft" {
			_ = json.Unmarshal(item["version"], &previousGame)
		}
	}
	items = slices.DeleteFunc(items, func(item map[string]json.RawMessage) bool {
		var uid string
		_ = json.Unmarshal(item["uid"], &uid)
		return ComponentLoader(uid) != "" || uid == "net.fabricmc.intermediary" || previousGame != game && (uid == "org.lwjgl" || uid == "org.lwjgl3" || uid == "net.minecraft.java" || uid == "net.java")
	})
	for _, item := range items {
		var uid string
		_ = json.Unmarshal(item["uid"], &uid)
		if uid == "net.minecraft" {
			item["version"], _ = json.Marshal(game)
			for key := range item {
				if strings.HasPrefix(key, "cached") {
					delete(item, key)
				}
			}
		}
	}
	if loaderID != "" {
		uid, _ := json.Marshal(LoaderUID(loaderID))
		v, _ := json.Marshal(version)
		items = append(items, map[string]json.RawMessage{"uid": uid, "version": v})
	}
	fields["components"], err = json.Marshal(items)
	if err != nil {
		return PrismPack{}, nil, err
	}
	raw, err = json.MarshalIndent(fields, "", "  ")
	var pack PrismPack
	if err == nil {
		err = json.Unmarshal(raw, &pack)
	}
	return pack, raw, err
}
