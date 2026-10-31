package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

type GameDirectory struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Directories struct {
	SelectedInstance string          `json:"selectedInstance"`
	InstanceOrder    []string        `json:"instanceOrder,omitempty"`
	Entries          []GameDirectory `json:"entries"`
}

func DataDir() string {
	home, _ := os.UserHomeDir()
	var root string
	switch runtime.GOOS {
	case "windows":
		root, _ = os.UserConfigDir()
	case "darwin":
		root = filepath.Join(home, "Library", "Application Support")
	default:
		root = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(root) {
			root = filepath.Join(home, ".local", "share")
		}
	}
	return absolute(filepath.Join(root, "tomori"))
}

func defaultDirectories() Directories {
	return Directories{Entries: []GameDirectory{{Name: "Tomori", Path: DefaultGameDirectory()}}}
}

func DefaultGameDirectory() string { return filepath.Join(DataDir(), "minecraft") }

func (d *Directories) normalize() {
	base := defaultDirectories()
	entries := base.Entries
	seen := map[string]bool{pathKey(DefaultGameDirectory()): true}
	for _, entry := range d.Entries {
		if strings.TrimSpace(entry.Path) == "" {
			continue
		}
		entry.Path = absolute(entry.Path)
		if seen[pathKey(entry.Path)] {
			continue
		}
		seen[pathKey(entry.Path)] = true
		if strings.TrimSpace(entry.Name) == "" {
			entry.Name = directoryName(entry.Path)
		}
		entries = append(entries, entry)
	}
	d.Entries = entries
}

func (d *Directories) Add(path, name string) error {
	path = absolute(path)
	for _, entry := range d.Entries {
		if pathKey(entry.Path) == pathKey(path) {
			return fmt.Errorf(locales.DirectoryAlreadyAdded(), path)
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = directoryName(path)
	}
	d.Entries = append(d.Entries, GameDirectory{Name: name, Path: path})
	return nil
}

func (d *Directories) Edit(original, path, name string) error {
	if pathKey(original) == pathKey(DefaultGameDirectory()) {
		return errors.New(locales.BuiltInGameDirectoryCannotBeChanged())
	}
	path = absolute(path)
	for _, entry := range d.Entries {
		if entry.Path != original && pathKey(entry.Path) == pathKey(path) {
			return fmt.Errorf(locales.DirectoryAlreadyAdded(), path)
		}
	}
	for i := range d.Entries {
		entry := &d.Entries[i]
		if entry.Path != original {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			name = directoryName(path)
		}
		entry.Name, entry.Path = name, path
		return nil
	}
	return errors.New(locales.DirectoryRemoved())
}

func directoryName(path string) string {
	name := filepath.Base(path)
	if name == ".minecraft" || name == "minecraft" {
		name = filepath.Base(filepath.Dir(path))
	}
	return name
}

func (c Config) CacheDirectory() string {
	if strings.TrimSpace(c.Download.CacheDirectory) != "" {
		return absolute(c.Download.CacheDirectory)
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = filepath.Join(DataDir(), "cache")
		return dir
	}
	return filepath.Join(dir, "tomori")
}

func absolute(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func pathKey(path string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
