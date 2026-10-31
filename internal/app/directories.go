package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/egoist/mygo"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) openDirectory(path string) {
	a.workers.Go(func() {
		err := os.MkdirAll(path, 0o755)
		if err == nil {
			err = mygo.Shell.OpenPath(path)
		}
		if err != nil {
			a.logf(locales.FailedToOpenFolder(), err)
		}
	})
}

func (a *app) openFile(path string) {
	a.workers.Go(func() {
		if err := mygo.Shell.OpenPath(path); err != nil {
			a.logf(locales.FailedToOpenFile(), err)
		}
	})
}

func (a *app) removeDirectory(path string) {
	if a.hasTasks() || a.pathBusy || len(a.games) > 0 || path == config.DefaultGameDirectory() {
		return
	}
	a.cfg.Directories.Entries = slices.DeleteFunc(a.cfg.Directories.Entries, func(entry config.GameDirectory) bool { return entry.Path == path })
	a.saveConfig()
}

func (a *app) addDirectories() {
	if a.hasTasks() || a.pathBusy || len(a.games) > 0 {
		return
	}
	a.openDirectoryEditor(config.GameDirectory{Path: config.DefaultGameDirectory()}, false)
}

func (a *app) editDirectory(path string) {
	if a.hasTasks() || a.pathBusy || len(a.games) > 0 || path == config.DefaultGameDirectory() {
		return
	}
	for _, entry := range a.cfg.Directories.Entries {
		if entry.Path == path {
			a.openDirectoryEditor(entry, true)
			return
		}
	}
}

// Accept a Minecraft root or a launcher folder containing a Minecraft root.
func resolveGameDirectory(path string) (string, error) {
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf(locales.NotADirectory(), path)
	}
	if info, err := os.Stat(filepath.Join(path, "versions")); err == nil && info.IsDir() {
		return path, nil
	}
	for _, name := range []string{".minecraft", "minecraft"} {
		child := filepath.Join(path, name)
		if info, err := os.Stat(filepath.Join(child, "versions")); err == nil && info.IsDir() {
			return child, nil
		}
	}
	return path, nil
}
