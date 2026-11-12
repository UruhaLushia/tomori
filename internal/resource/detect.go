package resource

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type LocalFile struct {
	Path string
	Kind Kind
}

// Detect reads archive entries without extracting their contents. An empty kind
// leaves ZIP archives to the modpack importer, which validates its own formats.
func Detect(ctx context.Context, filename string) (Kind, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	info, err := os.Stat(filename)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if level, err := os.Stat(filepath.Join(filename, "level.dat")); err == nil && level.Mode().IsRegular() {
			return World, nil
		}
		if shaders, err := os.Stat(filepath.Join(filename, "shaders")); err == nil && shaders.IsDir() {
			return ShaderPack, nil
		}
		if data, err := os.Stat(filepath.Join(filename, "data")); err == nil && data.IsDir() {
			if _, err := os.Stat(filepath.Join(filename, "pack.mcmeta")); err == nil {
				return DataPack, nil
			}
		}
		if _, err := os.Stat(filepath.Join(filename, "pack.mcmeta")); err == nil {
			return ResourcePack, nil
		}
		return "", errors.New(locales.SelectResourceFiles())
	}
	base := filepath.Base(filename)
	if strings.HasSuffix(strings.ToLower(base), ".disabled") {
		base = base[:len(base)-len(".disabled")]
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".jar", ".litemod":
		return Mod, nil
	case ".mrpack":
		return "", nil
	case ".zip":
	default:
		return "", errors.New(locales.UnsupportedFileType())
	}
	archive, err := fsutil.OpenZip(ctx, filename)
	if err != nil {
		return "", err
	}
	defer archive.Close()
	wrapper := ""
	for _, file := range archive.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		slash := strings.IndexByte(name, '/')
		if slash < 1 {
			wrapper = ""
			break
		}
		if wrapper == "" {
			wrapper = name[:slash]
		} else if wrapper != name[:slash] {
			wrapper = ""
			break
		}
	}
	var mod, pack, assets, data, shaders, world, modpack bool
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		name := path.Clean(strings.ReplaceAll(file.Name, "\\", "/"))
		if wrapper != "" {
			name = strings.TrimPrefix(name, wrapper+"/")
		}
		switch name {
		case "fabric.mod.json", "quilt.mod.json", "META-INF/mods.toml", "META-INF/neoforge.mods.toml", "mcmod.info", "litemod.json":
			mod = true
		case "pack.mcmeta", "pack.txt":
			pack = true
		}
		assets = assets || strings.HasPrefix(name, "assets/")
		data = data || strings.HasPrefix(name, "data/")
		shaders = shaders || strings.HasPrefix(name, "shaders/")
		if !strings.Contains(name, "/") {
			switch path.Base(name) {
			case "level.dat":
				world = true
			case "modrinth.index.json", "manifest.json", "mmc-pack.json", "mcbbs.packmeta", "modpack.json", "hmclpack.json":
				modpack = true
			}
		}
	}
	switch {
	case mod:
		return Mod, nil
	case modpack || ext == ".mrpack":
		return "", nil
	case world:
		return World, nil
	case shaders:
		return ShaderPack, nil
	case pack && data && !assets:
		return DataPack, nil
	case pack:
		return ResourcePack, nil
	default:
		return "", nil
	}
}
