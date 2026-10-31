package loader

import (
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/download"
)

// Reuse only files whose checksum is supplied by the loader installer.
func (i *Installer) reuseLibraries() download.LocalSources {
	if len(i.ReuseRoots) == 0 {
		return nil
	}
	return func(task download.Task) []string {
		rel, err := filepath.Rel(i.Root, task.Path)
		if err != nil || !filepath.IsLocal(rel) {
			return nil
		}
		sources := make([]string, 0, len(i.ReuseRoots))
		for _, root := range i.ReuseRoots {
			sources = append(sources, filepath.Join(root, rel))
		}
		return sources
	}
}
