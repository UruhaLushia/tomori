package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type forgeProfile struct {
	JSON      string `json:"json"`
	Minecraft string `json:"minecraft"`
	Data      map[string]struct {
		Client string `json:"client"`
	} `json:"data"`
	Libraries   []minecraft.Library             `json:"libraries"`
	Processors  []processor                     `json:"processors"`
	VersionInfo *minecraft.VersionDetail        `json:"versionInfo"`
	Install     struct{ Path, FilePath string } `json:"install"`
}

type processor struct {
	Jar                    string
	Classpath, Args, Sides []string
	Outputs                map[string]string
}

func (i *Installer) installForge(ctx context.Context, base *minecraft.VersionDetail, filename string) (*minecraft.VersionDetail, error) {
	z, err := fsutil.OpenZip(ctx, filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = z.Close() }()
	read := func(name string, target any) error {
		f, err := fsutil.FindZipFile(z.Reader, strings.TrimPrefix(name, "/"))
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf(locales.InstallerIsMissing(), name)
		}
		if err != nil {
			return err
		}
		data, err := fsutil.ReadZipFile(ctx, f, 16<<20)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, target)
	}
	var profile forgeProfile
	if err := read("install_profile.json", &profile); err != nil {
		return nil, err
	}
	if profile.Minecraft != "" && profile.Minecraft != base.ID {
		return nil, errors.New(locales.InstallerMinecraftVersionMismatch())
	}
	version := profile.VersionInfo
	if version == nil {
		version = &minecraft.VersionDetail{}
		if profile.JSON == "" {
			return nil, errors.New(locales.InstallerHasNoVersionInformation())
		}
		if err := read(profile.JSON, version); err != nil {
			return nil, err
		}
	}
	if version.InheritsFrom != "" && version.InheritsFrom != base.ID {
		return nil, errors.New(locales.ForgeParentVersionMismatch())
	}
	work, err := os.MkdirTemp(filepath.Join(i.Root), ".loader-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(work) }()
	libraries, err := fsutil.NewArchiveWriter(filepath.Join(i.Root, "libraries"), 200000, 4<<30, 8<<30)
	if err != nil {
		return nil, err
	}
	defer func() { _ = libraries.Close() }()
	data, err := fsutil.NewArchiveWriter(work, 10000, 256<<20, 1<<30)
	if err != nil {
		return nil, err
	}
	defer func() { _ = data.Close() }()
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		var name string
		writer := libraries
		switch {
		case strings.HasPrefix(f.Name, "maven/"):
			name = strings.TrimPrefix(f.Name, "maven/")
		case strings.HasPrefix(f.Name, "data/"):
			name, writer = f.Name, data
		case profile.Install.FilePath != "" && f.Name == profile.Install.FilePath:
			name, err = minecraft.MavenPath(profile.Install.Path)
		}
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		if err := writer.WriteZip(ctx, f, name); err != nil {
			return nil, err
		}
	}
	var tasks []download.Task
	for _, library := range append(profile.Libraries, version.Libraries...) {
		if !library.Applies(minecraft.CurrentEnvironment()) {
			continue
		}
		ref, err := library.Artifact()
		if err != nil {
			return nil, err
		}
		if ref == nil {
			continue
		}
		dst, err := fsutil.SafePath(filepath.Join(i.Root, "libraries"), ref.Path)
		if err != nil {
			return nil, err
		}
		if fsutil.Complete(dst, ref.SHA1, ref.Size) {
			continue
		}
		if ref.URL == "" {
			continue
		}
		tasks = append(tasks, download.Task{URL: ref.URL, Path: dst, SHA1: ref.SHA1, Size: ref.Size})
	}
	if err := i.Fetcher.FetchAllWithReuse(ctx, tasks, i.Fetcher.Workers, i.downloadProgress, i.reuseLibraries()); err != nil {
		return nil, err
	}
	vars := map[string]string{"SIDE": "client", "ROOT": i.Root, "LIBRARY_DIR": filepath.Join(i.Root, "libraries"), "MINECRAFT_JAR": filepath.Join(i.Root, "versions", base.ID, base.ID+".jar"), "MINECRAFT_VERSION": base.ID, "INSTALLER": filename}
	for key, value := range profile.Data {
		vars[key] = value.Client
	}
	for key := range profile.Data {
		value, err := i.value(vars[key], vars, work, 0)
		if err != nil {
			return nil, err
		}
		vars[key] = value
	}
	for index, p := range profile.Processors {
		// Download mappings through the launcher so retries, mirrors, progress
		// and cancellation use the same downloader as the rest of the install.
		if base.Downloads.ClientMappings.URL != "" && strings.Contains(strings.Join(p.Args, "\x00"), "DOWNLOAD_MOJMAPS") {
			for n, arg := range p.Args {
				if arg != "--output" || n+1 >= len(p.Args) {
					continue
				}
				dst, err := i.value(p.Args[n+1], vars, work, 0)
				if err != nil {
					return nil, err
				}
				if rel, err := filepath.Rel(i.Root, dst); err != nil || !filepath.IsLocal(rel) {
					return nil, errors.New(locales.MappingOutputEscapesInstanceDirectory())
				}
				ref := base.Downloads.ClientMappings
				if err := i.Fetcher.FetchAll(ctx, []download.Task{{URL: ref.URL, Path: dst, SHA1: ref.SHA1, Size: ref.Size}}, 1, i.downloadProgress); err != nil {
					return nil, err
				}
				goto nextProcessor
			}
		}
		if err := i.runProcessor(ctx, p, vars, work); err != nil {
			return nil, fmt.Errorf(locales.LoaderProcessor(), index+1, len(profile.Processors), err)
		}
	nextProcessor:
	}
	return minecraft.MergeVersion(base, version), nil
}

func (i *Installer) value(raw string, vars map[string]string, work string, depth int) (string, error) {
	if depth > 20 {
		return "", errors.New(locales.CircularInstallerVariableReference())
	}
	if strings.HasPrefix(raw, "'") && strings.HasSuffix(raw, "'") {
		return strings.Trim(raw, "'"), nil
	}
	for start := strings.IndexByte(raw, '{'); start >= 0; start = strings.IndexByte(raw, '{') {
		end := strings.IndexByte(raw[start:], '}')
		if end < 0 {
			return "", fmt.Errorf(locales.InvalidInstallerVariable(), raw)
		}
		end += start
		key := raw[start+1 : end]
		value, ok := vars[key]
		if !ok {
			return "", fmt.Errorf(locales.UnknownInstallerVariable(), key)
		}
		resolved, err := i.value(value, vars, work, depth+1)
		if err != nil {
			return "", err
		}
		raw = raw[:start] + resolved + raw[end+1:]
	}
	if strings.HasPrefix(raw, "[") && strings.HasSuffix(raw, "]") {
		p, err := minecraft.MavenPath(raw[1 : len(raw)-1])
		if err != nil {
			return "", err
		}
		return fsutil.SafePath(filepath.Join(i.Root, "libraries"), p)
	}
	if strings.HasPrefix(raw, "/data/") {
		return fsutil.SafePath(work, strings.TrimPrefix(raw, "/"))
	}
	return raw, nil
}
