package resource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func Inspect(ctx context.Context, path string, kind Kind) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return Entry{}, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(filepath.Base(path))
	if err != nil {
		return Entry{}, err
	}
	name := filepath.Base(path)
	entry := Entry{File: name, Relative: name, Name: strings.TrimSuffix(name, filepath.Ext(name)), Size: info.Size(), Directory: info.IsDir(), Enabled: true}
	err = readMetadata(ctx, root, name, kind, &entry)
	return entry, err
}

func readMetadata(ctx context.Context, root *os.Root, path string, kind Kind, entry *Entry) error {
	var read func(string) ([]byte, error)
	if entry.Directory {
		read = func(name string) ([]byte, error) {
			f, err := fsutil.OpenRootRegular(root, filepath.Join(path, name))
			if err != nil {
				return nil, err
			}
			defer func() { _ = f.Close() }()
			return readSmall(fsutil.ContextReader(ctx, f))
		}
	} else {
		f, err := fsutil.OpenRootRegular(root, path)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		info, err := f.Stat()
		if err != nil {
			return err
		}
		archive, err := fsutil.NewZipReader(ctx, f, info.Size())
		if err != nil {
			return fmt.Errorf(locales.CannotReadArchive(), err)
		}
		read = func(name string) ([]byte, error) {
			f, err := fsutil.FindZipFile(archive, name)
			if err != nil {
				return nil, err
			}
			return fsutil.ReadZipFile(ctx, f, 1<<20)
		}
	}
	if kind != Mod {
		raw, err := read("pack.mcmeta")
		if errors.Is(err, os.ErrNotExist) && kind != DataPack {
			return nil
		}
		if err != nil {
			return err
		}
		var pack struct {
			Pack *struct {
				Description json.RawMessage `json:"description"`
			} `json:"pack"`
		}
		if err := json.Unmarshal(raw, &pack); err != nil {
			return err
		}
		if pack.Pack == nil {
			return errors.New(locales.MissingPackMcmetaMetadata())
		}
		entry.Description = componentText(pack.Pack.Description)
		return nil
	}
	for _, name := range []string{"fabric.mod.json", "quilt.mod.json", "META-INF/neoforge.mods.toml", "META-INF/mods.toml", "mcmod.info", "litemod.json"} {
		raw, err := read(name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := parseModMetadata(name, raw, entry); err != nil {
			return err
		}
		if strings.Contains(entry.Version, "${") {
			entry.Version = ""
			if manifest, err := read("META-INF/MANIFEST.MF"); err == nil {
				for line := range strings.SplitSeq(string(manifest), "\n") {
					if value, ok := strings.CutPrefix(strings.TrimSpace(line), "Implementation-Version:"); ok {
						entry.Version = strings.TrimSpace(value)
						break
					}
				}
			}
		}
		return nil
	}
	return nil
}

func readSmall(r io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(r, (1<<20)+1))
	if len(raw) > 1<<20 {
		return nil, errors.New(locales.ResourceMetadataExceeds1MiB())
	}
	return raw, err
}

func parseModMetadata(name string, raw []byte, entry *Entry) error {
	var id, title, version, description, loader string
	switch name {
	case "fabric.mod.json", "litemod.json":
		var data struct{ ID, Name, Version, Description string }
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		id, title, version, description, loader = data.ID, data.Name, data.Version, data.Description, "Fabric"
		if name == "litemod.json" {
			loader = "LiteLoader"
		}
	case "quilt.mod.json":
		var data struct {
			Quilt struct {
				ID, Version string
				Metadata    struct{ Name, Description string }
			} `json:"quilt_loader"`
		}
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		id, title, version, description, loader = data.Quilt.ID, data.Quilt.Metadata.Name, data.Quilt.Version, data.Quilt.Metadata.Description, "Quilt"
	case "META-INF/mods.toml", "META-INF/neoforge.mods.toml":
		var data struct {
			Mods []struct {
				ID          string `toml:"modId"`
				Name        string `toml:"displayName"`
				Version     string `toml:"version"`
				Description string `toml:"description"`
			} `toml:"mods"`
		}
		if err := toml.Unmarshal(raw, &data); err != nil {
			return err
		}
		if len(data.Mods) == 0 {
			return nil
		}
		mod := data.Mods[0]
		id, title, version, description, loader = mod.ID, mod.Name, mod.Version, mod.Description, "Forge"
		if name == "META-INF/neoforge.mods.toml" {
			loader = "NeoForge"
		}
	case "mcmod.info":
		type mod struct {
			ID                         string `json:"modid"`
			Name, Version, Description string
		}
		var list []mod
		if err := json.Unmarshal(raw, &list); err != nil {
			var data struct {
				Mods []mod `json:"modList"`
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				return err
			}
			list = data.Mods
		}
		if len(list) == 0 {
			return nil
		}
		id, title, version, description, loader = list[0].ID, list[0].Name, list[0].Version, list[0].Description, "Forge"
	}
	if title != "" {
		entry.Name = title
	}
	entry.ID, entry.Version, entry.Description, entry.Loader = id, version, strings.TrimSpace(description), loader
	return nil
}

func componentText(raw json.RawMessage) string {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	var text strings.Builder
	pending := []any{value}
	for len(pending) > 0 {
		value = pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if object, ok := value.(map[string]any); ok {
			if part, ok := object["text"].(string); ok {
				text.WriteString(part)
			}
			value = object["extra"]
			if _, ok := value.([]any); !ok {
				continue
			}
		}
		switch value := value.(type) {
		case string:
			text.WriteString(value)
		case []any:
			for i := len(value) - 1; i >= 0; i-- {
				pending = append(pending, value[i])
			}
		}
	}
	return text.String()
}
