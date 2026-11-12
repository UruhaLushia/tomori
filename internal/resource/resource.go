package resource

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/i18n"
)

type Kind string

// InstallPolicy controls how an online install handles an existing project.
type InstallPolicy uint8

const (
	InstallDefault InstallPolicy = iota
	InstallReplace
	InstallKeepDisabled
)

// Compatibility describes the target instance for a local resource import.
// Empty fields leave that part unchecked when the resource has no metadata for it.
type Compatibility struct {
	GameVersion string
	Loader      string
	Loaders     []string
}

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
	GameVersions                                           []string
	Size                                                   int64
	Modified                                               time.Time
	Enabled, Directory                                     bool
	Problem                                                string
	Origins                                                []Origin
	TranslatedName, SearchAliases                          string
}

func (e Entry) Validate(target Compatibility) error {
	if target.GameVersion != "" && len(e.GameVersions) > 0 {
		matched := false
		for _, required := range e.GameVersions {
			if versionMatches(required, target.GameVersion) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s: %s", locales.IncompatibleVersion(), strings.Join(e.GameVersions, ", "))
		}
	}
	if e.Loader == "" {
		return nil
	}
	loaders := append([]string{}, target.Loaders...)
	if target.Loader != "" {
		loaders = append(loaders, target.Loader)
	}
	if len(loaders) == 0 {
		return nil
	}
	want := normalizeLoader(e.Loader)
	for _, loader := range loaders {
		if normalizeLoader(loader) == want {
			return nil
		}
	}
	return errors.New(locales.LoaderGameVersionMismatch())
}

func normalizeLoader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	for _, loader := range []string{"neoforge", "forge", "fabric", "quilt", "liteloader"} {
		if strings.Contains(value, loader) {
			return loader
		}
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func versionMatches(requirement, actual string) bool {
	requirement = strings.TrimSpace(requirement)
	actual = strings.TrimSpace(actual)
	if requirement == "" || requirement == "*" {
		return true
	}
	for _, alternative := range strings.Split(requirement, "||") {
		if versionMatchesRange(strings.TrimSpace(alternative), actual) {
			return true
		}
	}
	return false
}

func versionMatchesRange(requirement, actual string) bool {
	if strings.HasSuffix(requirement, ".*") {
		return strings.HasPrefix(actual, strings.TrimSuffix(requirement, ".*"))
	}
	if len(requirement) > 1 && (requirement[0] == '[' || requirement[0] == '(') && (requirement[len(requirement)-1] == ']' || requirement[len(requirement)-1] == ')') {
		bounds := strings.Split(requirement[1:len(requirement)-1], ",")
		if len(bounds) == 1 {
			return strings.EqualFold(strings.TrimSpace(bounds[0]), actual)
		}
		if len(bounds) == 2 {
			if lower := strings.TrimSpace(bounds[0]); lower != "" {
				comparison, ok := compareVersions(actual, lower)
				if !ok || comparison < 0 || comparison == 0 && requirement[0] == '(' {
					return false
				}
			}
			if upper := strings.TrimSpace(bounds[1]); upper != "" {
				comparison, ok := compareVersions(actual, upper)
				if !ok || comparison > 0 || comparison == 0 && requirement[len(requirement)-1] == ')' {
					return false
				}
			}
			return true
		}
	}
	for _, operator := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(requirement, operator) {
			bound := strings.TrimSpace(strings.TrimPrefix(requirement, operator))
			comparison, ok := compareVersions(actual, bound)
			if !ok {
				return false
			}
			switch operator {
			case ">=":
				return comparison >= 0
			case "<=":
				return comparison <= 0
			case ">":
				return comparison > 0
			default:
				return comparison < 0
			}
		}
	}
	return strings.EqualFold(requirement, actual)
}

func compareVersions(left, right string) (int, bool) {
	parse := func(value string) ([]int, bool) {
		parts := strings.FieldsFunc(value, func(r rune) bool { return r == '.' || r == '-' || r == '+' })
		if len(parts) == 0 {
			return nil, false
		}
		result := make([]int, len(parts))
		for i, part := range parts {
			end := 0
			for end < len(part) && part[end] >= '0' && part[end] <= '9' {
				end++
			}
			if end == 0 {
				return nil, false
			}
			result[i], _ = strconv.Atoi(part[:end])
		}
		return result, true
	}
	a, ok := parse(left)
	if !ok {
		return 0, false
	}
	b, ok := parse(right)
	if !ok {
		return 0, false
	}
	for i := 0; i < len(a) || i < len(b); i++ {
		var av, bv int
		if i < len(a) {
			av = a[i]
		}
		if i < len(b) {
			bv = b[i]
		}
		if av < bv {
			return -1, true
		}
		if av > bv {
			return 1, true
		}
	}
	return 0, true
}

func (e Entry) DisplayName() string {
	if !strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") && e.TranslatedName != "" && e.TranslatedName != e.Name {
		return e.TranslatedName + " · " + e.Name
	}
	return e.Name
}

const disabledDirectory = ".tomori-disabled"
