package app

import (
	"context"
	"os"

	"github.com/egoist/mygo"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func pickerLocations(ctx context.Context) []fsutil.DirectoryEntry {
	locations := fsutil.DirectoryLocations(ctx)
	seen := make(map[string]bool, len(locations)+3)
	for _, entry := range locations {
		seen[entry.Path] = true
	}
	common := []struct {
		name string
		path mygo.PathName
	}{
		{locales.Desktop(), mygo.PathDesktop},
		{locales.DownloadsPage(), mygo.PathDownloads},
		{locales.Documents(), mygo.PathDocuments},
	}
	var shortcuts []fsutil.DirectoryEntry
	for _, location := range common {
		if ctx.Err() != nil {
			break
		}
		path, err := mygo.App.Path(location.path)
		if err != nil || path == "" {
			continue
		}
		if seen[path] {
			continue
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			shortcuts = append(shortcuts, fsutil.DirectoryEntry{Name: location.name, Path: path})
			seen[path] = true
		}
	}
	if len(locations) == 0 {
		return shortcuts
	}
	result := make([]fsutil.DirectoryEntry, 0, len(locations)+len(shortcuts))
	result = append(result, locations[0])
	result = append(result, shortcuts...)
	return append(result, locations[1:]...)
}
