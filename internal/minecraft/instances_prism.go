package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type PrismComponent struct {
	UID      string `json:"uid"`
	Version  string `json:"version"`
	Order    *int   `json:"order,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
}

type PrismPack struct {
	FormatVersion int              `json:"formatVersion"`
	Components    []PrismComponent `json:"components"`
}

func ReadPrismPack(path string) (PrismPack, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return PrismPack{}, err
	}
	defer root.Close()
	return readPrismPack(root)
}

func readPrismPack(root *os.Root) (PrismPack, error) {
	var pack PrismPack
	raw, err := fsutil.ReadRootFile(root, "mmc-pack.json", 4<<20)
	if err != nil {
		return pack, err
	}
	if err := json.Unmarshal(raw, &pack); err != nil {
		return pack, fmt.Errorf(locales.ReadingMmcPackJson(), err)
	}
	return pack, validatePrismPack(pack)
}

func validatePrismPack(pack PrismPack) error {
	if pack.FormatVersion != 1 {
		return fmt.Errorf(locales.UnsupportedPrismInstanceFormat(), pack.FormatVersion)
	}
	seen := make(map[string]bool)
	for _, component := range pack.Components {
		if component.Disabled {
			continue
		}
		if err := fsutil.ValidateID(component.UID); err != nil {
			return err
		}
		if err := fsutil.ValidateID(component.Version); err != nil {
			return err
		}
		if seen[component.UID] {
			return fmt.Errorf(locales.DuplicatePrismComponent(), component.UID)
		}
		seen[component.UID] = true
	}
	return nil
}

func discoverPrismInstances(ctx context.Context, root string) ([]Instance, []error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	// Accept an instance, an instances directory, or Prism's data directory.
	if _, err := os.Stat(filepath.Join(root, "mmc-pack.json")); err == nil {
		instance, err := ReadPrismInstance(root)
		if err != nil {
			return nil, []error{fmt.Errorf("%s: %w", root, err)}, nil
		}
		return []Instance{instance}, nil, nil
	}
	if info, err := os.Stat(filepath.Join(root, "instances")); err == nil && info.IsDir() {
		root = filepath.Join(root, "instances")
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var list []Instance
	var problems []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, problems, err
		}
		if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		instance, err := ReadPrismInstance(filepath.Join(root, entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", entry.Name(), err))
			continue
		}
		list = append(list, instance)
	}
	return list, problems, nil
}

func ReadPrismInstance(path string) (Instance, error) {
	root, err := os.OpenRoot(path)
	if err != nil {
		return Instance{}, err
	}
	defer root.Close()
	pack, err := readPrismPack(root)
	if err != nil {
		return Instance{}, err
	}
	settings, err := fsutil.ReadRootFile(root, "instance.cfg", 1<<20)
	if err != nil {
		return Instance{}, err
	}
	name := filepath.Base(path)
	iconKey := ""
	general := false
	for line := range strings.SplitSeq(string(settings), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			general = line == "[General]"
			continue
		}
		if !general {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			if decoded, err := strconv.Unquote(value); err == nil {
				value = decoded
			}
			if strings.TrimSpace(value) != "" {
				name = strings.TrimSpace(value)
			}
		case "iconKey":
			iconKey = strings.TrimSpace(value)
		}
	}
	instance := Instance{
		ID: "prism:" + filepath.Base(path), Type: "prism",
		Name: name, InstanceDir: path, Format: InstancePrism,
		GameDir: filepath.Join(path, "minecraft"),
		IconKey: iconKey,
	}
	if info, err := os.Stat(instance.GameDir); err != nil || !info.IsDir() {
		if info, err := os.Stat(filepath.Join(path, ".minecraft")); err == nil && info.IsDir() {
			instance.GameDir = filepath.Join(path, ".minecraft")
		}
	}
	var loaders []string
	for _, component := range pack.Components {
		if component.Disabled {
			continue
		}
		if component.UID == "net.minecraft" {
			instance.GameVersion = component.Version
		}
		label := map[string]string{
			"net.minecraftforge": "Forge", "net.neoforged": "NeoForge",
			"net.fabricmc.fabric-loader": "Fabric", "org.quiltmc.quilt-loader": "Quilt",
			"com.mumfrey.liteloader": "LiteLoader",
		}[component.UID]
		if label != "" {
			loaders = append(loaders, label+" "+component.Version)
			switch component.UID {
			case "net.minecraftforge":
				instance.Loaders = append(instance.Loaders, "forge")
			case "net.neoforged":
				instance.Loaders = append(instance.Loaders, "neoforge")
			case "net.fabricmc.fabric-loader":
				instance.Loaders = append(instance.Loaders, "fabric")
			case "org.quiltmc.quilt-loader":
				instance.Loaders = append(instance.Loaders, "quilt")
			}
		}
	}
	if instance.GameVersion == "" {
		return Instance{}, errors.New(locales.MissingMinecraftVersion())
	}
	instance.Loader = strings.Join(loaders, " · ")
	if raw, err := fsutil.ReadRootFile(root, PrismMetadataFile, 1<<20); err == nil {
		var meta PrismMetadata
		if json.Unmarshal(raw, &meta) == nil {
			instance.Description, instance.PackFormat, instance.PackVersion = meta.Description, meta.PackFormat, meta.PackVersion
			instance.PackSource = meta.PackSource
		}
	}
	return instance, nil
}
