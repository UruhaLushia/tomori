package minecraft

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const instanceDisplayFile = ".tomori-display.json"
const instanceIconFile = ".tomori-icon.png"

func OpenInstanceRoot(ctx context.Context, instance Instance) (*os.Root, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := filepath.EvalSymlinks(instance.InstanceDir)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(instance.InstanceDir) != path || filepath.Dir(path) == path {
		return nil, errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == path {
		return nil, errors.New(locales.TheHomeDirectoryCannotBeModified())
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, err
	}
	var name string
	switch instance.Format {
	case InstanceManaged:
		name = InstanceMetadata
	case InstancePrism:
		name = "mmc-pack.json"
	case InstanceMinecraft:
		if err := fsutil.ValidateID(instance.ID); err != nil {
			_ = root.Close()
			return nil, err
		}
		expected, resolveErr := filepath.EvalSymlinks(filepath.Join(instance.GameDir, "versions", instance.ID))
		if resolveErr != nil || expected != path {
			_ = root.Close()
			return nil, errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
		}
		name = instance.ID + ".json"
	default:
		_ = root.Close()
		return nil, errors.New(locales.UnsupportedInstanceFormat())
	}
	raw, err := fsutil.ReadRootFile(root, name, 16<<20)
	if err == nil {
		var identity struct {
			ID, Version           string
			Format, FormatVersion int
		}
		err = json.Unmarshal(raw, &identity)
		if err == nil {
			valid := instance.Format == InstanceManaged && identity.Format == 1 && identity.Version == instance.ID ||
				instance.Format == InstancePrism && identity.FormatVersion == 1 ||
				instance.Format == InstanceMinecraft && (identity.ID == "" || identity.ID == instance.ID)
			if !valid {
				err = errors.New(locales.InstanceDirectoryChangedRefreshAndTryAgain())
			}
		}
	}
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

func RenameInstance(ctx context.Context, instance Instance, name string) error {
	if strings.ContainsFunc(name, unicode.IsControl) {
		return errors.New(locales.Use1128CharactersWithoutControlCharacters())
	}
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) || name == "" || utf8.RuneCountInString(name) > 128 {
		return errors.New(locales.Use1128CharactersWithoutControlCharacters())
	}
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	file := instanceDisplayFile
	var raw []byte
	switch instance.Format {
	case InstancePrism:
		file = "instance.cfg"
		raw, err = fsutil.ReadRootFile(root, file, 1<<20)
		if err == nil {
			raw = renamePrism(raw, name)
		}
	default:
		if instance.Format == InstanceManaged {
			file = InstanceMetadata
		}
		var fields map[string]json.RawMessage
		raw, err = fsutil.ReadRootFile(root, file, 1<<20)
		if errors.Is(err, os.ErrNotExist) && instance.Format == InstanceMinecraft {
			err = nil
		}
		if err == nil && len(raw) > 0 {
			err = json.Unmarshal(raw, &fields)
		}
		if err == nil {
			if fields == nil {
				fields = make(map[string]json.RawMessage)
			}
			fields["name"], _ = json.Marshal(name)
			raw, err = json.MarshalIndent(fields, "", "  ")
		}
	}
	if err != nil {
		return err
	}
	return fsutil.WriteRootAtomic(ctx, root, file, raw)
}

func renamePrism(raw []byte, name string) []byte {
	newline := "\n"
	if bytes.Contains(raw, []byte("\r\n")) {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	general, found, insert := false, false, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			general = trimmed == "[General]"
			if general {
				insert = i + 1
			}
			continue
		}
		if key, _, ok := strings.Cut(trimmed, "="); general && ok && strings.TrimSpace(key) == "name" {
			lines[i], found = "name="+strconv.Quote(name), true
		}
	}
	if !found {
		if insert < 0 {
			lines = append(lines, "[General]", "name="+strconv.Quote(name))
		} else {
			lines = append(lines, "")
			copy(lines[insert+1:], lines[insert:])
			lines[insert] = "name=" + strconv.Quote(name)
		}
	}
	return []byte(strings.Join(lines, newline))
}

func SetInstanceIcon(ctx context.Context, instance Instance, source string) error {
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if source == "" {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := root.Remove(instanceIconFile)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	icon, err := ReadInstanceIcon(ctx, source)
	if err != nil {
		return err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, icon); err != nil {
		return err
	}
	return fsutil.WriteRootAtomic(ctx, root, instanceIconFile, encoded.Bytes())
}

func InstanceTrashPath(ctx context.Context, instance Instance) (string, error) {
	root, err := OpenInstanceRoot(ctx, instance)
	if err != nil {
		return "", err
	}
	_ = root.Close()
	if instance.Format == InstanceMinecraft {
		entries, err := os.ReadDir(filepath.Join(instance.GameDir, "versions"))
		if err != nil {
			return "", err
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if entry.Name() == instance.ID || !entry.IsDir() {
				continue
			}
			r, err := os.OpenRoot(filepath.Join(instance.GameDir, "versions", entry.Name()))
			if err != nil {
				return "", err
			}
			raw, err := fsutil.ReadRootFile(r, entry.Name()+".json", 16<<20)
			_ = r.Close()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			var refs struct{ InheritsFrom, Jar string }
			if err := json.Unmarshal(raw, &refs); err != nil {
				return "", err
			}
			if refs.InheritsFrom == instance.ID || refs.Jar == instance.ID {
				return "", fmt.Errorf(locales.InstanceStillDependsOnThisVersion(), entry.Name())
			}
		}
	}
	return instance.InstanceDir, ctx.Err()
}
