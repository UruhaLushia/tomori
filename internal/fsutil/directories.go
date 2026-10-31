package fsutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

type DirectoryEntry struct {
	Name      string
	Path      string
	Directory bool
}

func DirectoryPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New(locales.EnterDirectoryPath())
	}
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}

func ReadEntries(ctx context.Context, path string, parentFallback, files bool, extensions []string) (string, []DirectoryEntry, error) {
	path, err := DirectoryPath(path)
	if err != nil {
		return "", nil, err
	}
	var entries []os.DirEntry
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		entries, err = os.ReadDir(path)
		if !parentFallback || !os.IsNotExist(err) || filepath.Dir(path) == path {
			break
		}
		path = filepath.Dir(path)
	}
	if err != nil {
		return "", nil, err
	}
	var directories []DirectoryEntry
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		child := filepath.Join(path, entry.Name())
		isDir := entry.IsDir()
		if !isDir && entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(child)
			isDir = err == nil && info.IsDir()
		}
		allowed := isDir
		if files && !isDir {
			for _, extension := range extensions {
				if strings.HasSuffix(strings.ToLower(entry.Name()), extension) {
					allowed = true
					break
				}
			}
		}
		if allowed {
			directories = append(directories, DirectoryEntry{Name: entry.Name(), Path: child, Directory: isDir})
		}
	}
	slices.SortStableFunc(directories, func(a, b DirectoryEntry) int {
		if a.Directory != b.Directory {
			if a.Directory {
				return -1
			}
			return 1
		}
		return CompareFold(a.Name, b.Name)
	})
	return path, directories, nil
}

func DirectoryLocations(ctx context.Context) []DirectoryEntry {
	var locations []DirectoryEntry
	if home, err := os.UserHomeDir(); err == nil {
		locations = append(locations, DirectoryEntry{Name: locales.Home(), Path: home})
	}
	if runtime.GOOS == "windows" {
		for drive := 'A'; drive <= 'Z'; drive++ {
			if ctx.Err() != nil {
				break
			}
			path := fmt.Sprintf("%c:\\", drive)
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				locations = append(locations, DirectoryEntry{Name: fmt.Sprintf("%c:", drive), Path: path})
			}
		}
	} else {
		locations = append(locations, DirectoryEntry{Name: locales.Filesystem(), Path: string(filepath.Separator)})
	}
	return locations
}
