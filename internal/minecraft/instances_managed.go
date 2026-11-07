package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const InstanceMetadata = "tomori-instance.json"

type ManagedMetadata struct {
	Format      int        `json:"format"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Version     string     `json:"version"`
	GameVersion string     `json:"gameVersion"`
	Loader      string     `json:"loader,omitempty"`
	Loaders     []string   `json:"loaders,omitempty"`
	PackFormat  string     `json:"packFormat,omitempty"`
	PackVersion string     `json:"packVersion,omitempty"`
	PackSource  PackSource `json:"packSource,omitzero"`
	ImportedAt  time.Time  `json:"importedAt"`
}

func WriteManaged(path string, meta ManagedMetadata) error {
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteAtomic(filepath.Join(path, InstanceMetadata), raw)
}

func ReadManaged(path string) (Instance, error) {
	instanceRoot, err := os.OpenRoot(path)
	if err != nil {
		return Instance{}, err
	}
	raw, err := fsutil.ReadRootFile(instanceRoot, InstanceMetadata, 1<<20)
	_ = instanceRoot.Close()
	if err != nil {
		return Instance{}, err
	}
	var m ManagedMetadata
	if err := json.Unmarshal(raw, &m); err != nil {
		return Instance{}, err
	}
	if m.Format != 1 {
		return Instance{}, errors.New(locales.UnsupportedInstanceFormat())
	}
	if err := fsutil.ValidateID(m.Version); err != nil {
		return Instance{}, err
	}
	root := filepath.Join(path, "minecraft")
	v, err := ReadLocalVersion(root, m.Version)
	if err != nil {
		return Instance{}, err
	}
	if v.MinecraftVersion != "" {
		m.GameVersion = v.MinecraftVersion
		m.Loaders, m.Loader = localLoaders(v, m.GameVersion)
	}
	return Instance{ID: m.Version, Type: "modpack", Time: m.ImportedAt, ReleaseTime: m.ImportedAt, Name: m.Name, Description: m.Description, GameVersion: m.GameVersion, Loader: m.Loader, Loaders: m.Loaders, GameDir: root, InstanceDir: path, Format: InstanceManaged, PackFormat: m.PackFormat, PackVersion: m.PackVersion, PackSource: m.PackSource}, v.Validate()
}

func discoverManagedInstances(ctx context.Context, root string) ([]Instance, []error, error) {
	if _, err := os.Stat(filepath.Join(root, InstanceMetadata)); err == nil {
		v, err := ReadManaged(root)
		if err != nil {
			return nil, []error{err}, nil
		}
		return []Instance{v}, nil, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "instances"))
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
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		instance, err := ReadManaged(filepath.Join(root, "instances", entry.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			problems = append(problems, fmt.Errorf("%s：%w", entry.Name(), err))
			continue
		}
		list = append(list, instance)
	}
	return list, problems, nil
}
