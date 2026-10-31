package resource

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/UruhaLushia/tomori/locales"
)

type Kind string

// InstallPolicy controls how an online install handles an existing project.
type InstallPolicy uint8

const (
	InstallDefault InstallPolicy = iota
	InstallReplace
	InstallKeepDisabled
)

const (
	Mod          Kind = "mods"
	ResourcePack Kind = "resourcepacks"
	ShaderPack   Kind = "shaderpacks"
	DataPack     Kind = "datapacks"
	World        Kind = "worlds"
)

func (k Kind) Valid() bool {
	return k == Mod || k == ResourcePack || k == ShaderPack || k == DataPack || k == World
}

func (k Kind) Label() string {
	switch k {
	case Mod:
		return locales.Mods()
	case ResourcePack:
		return locales.ResourcePacks()
	case ShaderPack:
		return locales.Shaders()
	case DataPack:
		return locales.DataPacks()
	case World:
		return locales.Worlds()
	}
	return locales.Resources()
}

func (k Kind) ProjectType() string {
	switch k {
	case Mod:
		return "mod"
	case ResourcePack:
		return "resourcepack"
	case ShaderPack:
		return "shader"
	case DataPack:
		return "datapack"
	case World:
		return "world"
	}
	return ""
}

func (k Kind) Accepts(name string, directory bool) bool {
	if !k.Valid() || strings.HasPrefix(name, ".tomori-") {
		return false
	}
	if k != DataPack {
		name = strings.TrimSuffix(name, ".disabled")
	}
	if directory {
		return k != Mod && k != World && !strings.HasPrefix(name, ".")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if k == World {
		return ext == ".zip"
	}
	if k == Mod {
		return ext == ".jar" || ext == ".zip" || ext == ".litemod"
	}
	return ext == ".zip"
}

func Directory(root string, kind Kind) (string, error) {
	if !kind.Valid() || root == "" {
		return "", errors.New(locales.InvalidResourceDirectory())
	}
	if kind == World {
		return filepath.Join(root, "saves"), nil
	}
	return filepath.Join(root, string(kind)), nil
}

type Entry struct {
	File, Relative, Name, ID, Version, Description, Loader string
	Size                                                   int64
	Modified                                               time.Time
	Enabled, Directory                                     bool
	Problem                                                string
	Origins                                                []Origin
	TranslatedName, SearchAliases                          string
}

func (e Entry) DisplayName() string {
	if !strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") && e.TranslatedName != "" && e.TranslatedName != e.Name {
		return e.TranslatedName + " · " + e.Name
	}
	return e.Name
}

const disabledDirectory = ".tomori-disabled"
