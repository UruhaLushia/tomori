package minecraft

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type Installer struct {
	Fetcher   *download.Client
	GameDir   string
	Isolate   bool
	Env       Environment
	AssetBase string
	Progress  func(Progress)
	// External launcher files are copied only after checksum verification.
	ReuseRoots    []string
	ClientSources []string
	ContentDir    string
	// Shared launcher files are checked and repaired in their original location.
	LibraryRoot, AssetRoot, ClientPath string
}

type Progress struct {
	Stage     string
	Done      int64
	Total     int64
	Files     int
	FilesDone int
	Download  *download.Progress
	Space     *storage.Report
}

func (i *Installer) report(p Progress) {
	if i.Progress != nil {
		i.Progress(p)
	}
}

func (i *Installer) VersionDir(id string) string {
	return filepath.Join(i.GameDir, "versions", id)
}

func (i *Installer) ClientJar(id string) string {
	if i.ClientPath != "" {
		return i.ClientPath
	}
	return filepath.Join(i.VersionDir(id), id+".jar")
}

func (i *Installer) NativesDir(id string) string {
	env := i.Env
	if env.OS == "" || env.Arch == "" {
		env = CurrentEnvironment()
	}
	platform := env.OS + "-" + nativeArch(env.Arch)
	if err := fsutil.ValidateID(platform); err != nil {
		platform = "unknown"
	}
	return filepath.Join(i.VersionDir(id), "natives-"+platform)
}

func (i *Installer) LibrariesDir() string {
	if i.LibraryRoot != "" {
		return i.LibraryRoot
	}
	return filepath.Join(i.GameDir, "libraries")
}

func (i *Installer) AssetsDir() string {
	if i.AssetRoot != "" {
		return i.AssetRoot
	}
	return filepath.Join(i.GameDir, "assets")
}

func (i *Installer) Install(ctx context.Context, detail *VersionDetail) error {
	return i.install(ctx, detail, true)
}

// Repair verifies the resolved launch files without rewriting the source
// profile, which may contain another launcher's patches or custom metadata.
func (i *Installer) Repair(ctx context.Context, detail *VersionDetail) error {
	return i.install(ctx, detail, false)
}

func (i *Installer) install(ctx context.Context, detail *VersionDetail, publish bool) error {
	if err := detail.Validate(); err != nil {
		return err
	}
	if detail.Downloads.Client.URL == "" && (publish || !fsutil.CompleteHashesContext(ctx, i.ClientJar(detail.ClientID()), detail.Downloads.Client.SHA1, "", detail.Downloads.Client.Size)) {
		return fmt.Errorf(locales.HasNoVanillaClientDownloadInformation(), detail.ID)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := i.VersionDir(detail.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	i.report(Progress{Stage: "prepare"})
	libTasks, nativeTasks, err := i.libraryTasks(detail)
	if err != nil {
		return err
	}
	assetTasks, err := i.assetTasks(ctx, detail)
	if err != nil {
		return err
	}
	tasks := append(i.clientTasks(detail), libTasks...)
	tasks = append(tasks, nativeTasks...)
	tasks = append(tasks, i.loggingTasks(detail)...)
	tasks = append(tasks, assetTasks...)
	space, err := i.Fetcher.CheckSpace(ctx, tasks)
	i.report(Progress{Stage: "space", Space: &space})
	if err != nil {
		return err
	}
	if err := i.Fetcher.FetchAllWithReuse(ctx, tasks, i.Fetcher.Workers, func(p download.Progress) {
		i.report(Progress{Stage: "download", Done: p.Done, Total: p.Total, Files: p.Files, FilesDone: p.FilesDone, Download: &p})
	}, i.reuseSources(detail)); err != nil {
		return err
	}
	i.report(Progress{Stage: "extract"})
	if err := i.extractNatives(ctx, detail); err != nil {
		return err
	}
	if err := i.linkLegacyAssets(ctx, detail); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	// Publish the version only after every dependency is usable.
	if publish {
		raw, err := json.MarshalIndent(detail, "", "  ")
		if err != nil {
			return err
		}
		if err := fsutil.WriteAtomic(filepath.Join(dir, detail.ID+".json"), raw); err != nil {
			return err
		}
	}
	i.report(Progress{Stage: "done"})
	return nil
}

func (i *Installer) clientTasks(detail *VersionDetail) []download.Task {
	if detail.Downloads.Client.URL == "" {
		return nil
	}
	return []download.Task{{
		URL:  detail.Downloads.Client.URL,
		Path: i.ClientJar(detail.ClientID()),
		SHA1: detail.Downloads.Client.SHA1,
		Size: detail.Downloads.Client.Size,
	}}
}

func (i *Installer) loggingTasks(detail *VersionDetail) []download.Task {
	if detail.Logging == nil || detail.Logging.Client == nil {
		return nil
	}
	c := detail.Logging.Client
	if c.File.URL == "" {
		return nil
	}
	return []download.Task{{
		URL:  c.File.URL,
		Path: filepath.Join(i.AssetsDir(), "log_configs", c.File.ID()),
		SHA1: c.File.SHA1,
		Size: c.File.Size,
	}}
}
