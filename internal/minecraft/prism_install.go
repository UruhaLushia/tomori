package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const PrismMetadataFile = ".tomori-pack.json"

type PrismMetadata struct {
	Description string     `json:"description,omitempty"`
	PackFormat  string     `json:"packFormat,omitempty"`
	PackVersion string     `json:"packVersion,omitempty"`
	PackSource  PackSource `json:"packSource,omitzero"`
}

func WritePrismMetadata(path string, meta PrismMetadata) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(filepath.Join(path, PrismMetadataFile), raw)
}

// PrismInstallRoot accepts a Prism data directory or its instances directory.
// An individual instance is never an installation destination.
func PrismInstallRoot(ctx context.Context, root string) (string, bool, error) {
	if _, err := os.Stat(filepath.Join(root, "mmc-pack.json")); err == nil {
		return "", false, errors.New(locales.SelectPrismSInstancesDirectoryInsteadOfAnIndividualInstance())
	}
	for _, dir := range []string{root, filepath.Dir(root)} {
		for _, name := range []string{"prismlauncher.cfg", "multimc.cfg"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				if dir == root {
					return filepath.Join(root, "instances"), true, nil
				}
				if filepath.Base(root) == "instances" {
					return root, true, nil
				}
			}
		}
	}
	for _, parent := range []string{filepath.Join(root, "instances"), root} {
		entries, err := os.ReadDir(parent)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", false, err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return "", false, err
			}
			if strings.HasPrefix(entry.Name(), ".") || !entry.IsDir() {
				continue
			}
			if _, err := os.Stat(filepath.Join(parent, entry.Name(), "mmc-pack.json")); err == nil {
				return parent, true, nil
			}
		}
	}
	return "", false, ctx.Err()
}

func WritePrismInstance(path, name, game string, components []PrismComponent) error {
	if err := fsutil.ValidateID(game); err != nil {
		return err
	}
	pack := PrismPack{FormatVersion: 1, Components: slices.Clone(components)}
	if index := slices.IndexFunc(pack.Components, func(component PrismComponent) bool { return component.UID == "net.minecraft" }); index >= 0 {
		pack.Components[index].Version = game
	} else {
		pack.Components = append([]PrismComponent{{UID: "net.minecraft", Version: game}}, pack.Components...)
	}
	raw, err := json.MarshalIndent(pack, "", "  ")
	if err != nil {
		return err
	}
	if err := fsutil.WriteAtomic(filepath.Join(path, "mmc-pack.json"), raw); err != nil {
		return err
	}
	// Qt INI strings escape control characters and backslashes.
	name = strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\r", "\\r", "\"", "\\\"").Replace(name)
	cfg := "[General]\nInstanceType=OneSix\nname=\"" + name + "\"\niconKey=default\n"
	return fsutil.WriteAtomic(filepath.Join(path, "instance.cfg"), []byte(cfg))
}
