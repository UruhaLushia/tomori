package minecraft

import (
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/download"
)

func (i *Installer) reuseSources(detail *VersionDetail) download.LocalSources {
	if len(i.ReuseRoots) == 0 && len(i.ClientSources) == 0 {
		return nil
	}
	return func(task download.Task) []string {
		rel, err := filepath.Rel(i.GameDir, task.Path)
		var sources []string
		if detail != nil && task.Path == i.ClientJar(detail.ClientID()) {
			sources = append(sources, i.ClientSources...)
			rel, err = filepath.Join("versions", detail.ClientID(), detail.ClientID()+".jar"), nil
		} else {
			for _, dir := range []struct{ path, name string }{{i.LibrariesDir(), "libraries"}, {i.AssetsDir(), "assets"}} {
				if path, pathErr := filepath.Rel(dir.path, task.Path); pathErr == nil && filepath.IsLocal(path) {
					rel, err = filepath.Join(dir.name, path), nil
					break
				}
			}
		}
		if err != nil || !filepath.IsLocal(rel) {
			return sources
		}
		for _, root := range i.ReuseRoots {
			sources = append(sources, filepath.Join(root, rel))
		}
		return sources
	}
}
