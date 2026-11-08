package modpack

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

// Extract applies common overrides before client overrides, without extracting
// launcher executables, accounts or version files into the game content.
func (a *Archive) Extract(ctx context.Context, dir string, progress func(int, int)) error {
	return a.ExtractRecorded(ctx, dir, progress, nil)
}

func (a *Archive) ExtractRecorded(ctx context.Context, dir string, progress func(int, int), record func(InstalledFile) error) error {
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: dir, Bytes: a.Info.ExpandedSize}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	writer, err := fsutil.NewArchiveWriter(dir, 200000, 4<<30, 64<<30)
	if err != nil {
		return err
	}
	defer func() { _ = writer.Close() }()
	return a.extractRecorded(ctx, writer, progress, record)
}

func (a *Archive) ExtractRoot(ctx context.Context, root *os.Root, progress func(int, int), record func(InstalledFile) error) error {
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: root.Name(), Bytes: a.Info.ExpandedSize}})
	if err != nil {
		return err
	}
	if err := space.Check(); err != nil {
		return err
	}
	writer, err := fsutil.NewRootArchiveWriter(root, 200000, 4<<30, 64<<30)
	if err != nil {
		return err
	}
	defer writer.Close()
	return a.extractRecorded(ctx, writer, progress, record)
}

type contentWriter interface {
	WriteZip(context.Context, *zip.File, string) error
	WriteZipSHA1(context.Context, *zip.File, string, string) (string, error)
}

func (a *Archive) extractRecorded(ctx context.Context, writer contentWriter, progress func(int, int), record func(InstalledFile) error) error {
	contents, err := a.contentEntries(ctx)
	if err != nil {
		return err
	}
	done := 0
	casePaths := make(map[string]string)
	for _, entries := range contents {
		seen := make(map[string]string)
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			relative, f := entry.relative, entry.file
			key := strings.ToLower(relative)
			if old, exists := casePaths[key]; exists && old != relative {
				return fmt.Errorf(locales.ModpackPathCaseConflict(), old, relative)
			}
			casePaths[key] = relative
			if old, exists := seen[key]; exists {
				return fmt.Errorf(locales.ModpackPathConflict(), old, relative)
			}
			seen[key] = relative
			expected := a.bundled[relative]
			if record == nil && expected == "" {
				err = writer.WriteZip(ctx, f, relative)
			} else {
				var sum string
				sum, err = writer.WriteZipSHA1(ctx, f, relative, expected)
				if err == nil && record != nil {
					err = record(InstalledFile{Path: relative, SHA1: sum, Size: int64(f.UncompressedSize64), Override: true})
				}
			}
			if err != nil {
				return err
			}
			done++
			if progress != nil {
				progress(done, len(a.names))
			}
		}
	}
	for name := range a.bundled {
		if err := ctx.Err(); err != nil {
			return err
		}
		if casePaths[strings.ToLower(name)] != name {
			return fmt.Errorf(locales.ModpackFileMissingOrChecksumMismatch(), name)
		}
	}
	return a.Verify()
}

func (a *Archive) contentPath(prefix, name string) (string, bool, error) {
	if !strings.HasPrefix(name, prefix) {
		return "", false, nil
	}
	relative := strings.TrimPrefix(name, prefix)
	if relative == "" || a.entries[name].FileInfo().IsDir() || a.Info.Format == "HMCL" && relative == "pack.json" {
		return "", false, nil
	}
	first, _, _ := strings.Cut(relative, "/")
	if (prefix == a.prefix || a.Info.Format == "Minecraft") && excludedContent(first) {
		return "", false, nil
	}
	return relative, true, validContentPath(relative)
}

func (a *Archive) contentEntries(ctx context.Context) ([][]archiveContent, error) {
	if a.contents != nil {
		return a.contents, nil
	}
	contents := make([][]archiveContent, len(a.overrides))
	for index, prefix := range a.overrides {
		for _, name := range a.names {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			relative, include, err := a.contentPath(prefix, name)
			if err != nil {
				return nil, err
			}
			if include {
				contents[index] = append(contents[index], archiveContent{file: a.entries[name], relative: relative})
			}
		}
	}
	a.contents = contents
	return contents, nil
}

func (a *Archive) expandedSize(ctx context.Context) (uint64, error) {
	contents, err := a.contentEntries(ctx)
	if err != nil {
		return 0, err
	}
	var size uint64
	for _, entries := range contents {
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			file := entry.file
			if file.UncompressedSize64 > 4<<30 || size > (64<<30)-file.UncompressedSize64 {
				return 0, errors.New(locales.ExtractedSizeExceedsTheLimit())
			}
			size += file.UncompressedSize64
		}
	}
	return size, nil
}

func excludedContent(name string) bool {
	switch strings.ToLower(name) {
	case "versions", "libraries", "assets", "launcher_profiles.json", "launcher_accounts.json", "launcher_settings.json", "accounts.json", "pack.json", "modrinth.index.json", "manifest.json", "modpack.json", "mcbbs.packmeta", "mmc-pack.json", "instance.cfg", "pcl", "hmcl", "logs", "crash-reports", "natives", ".tomori-disabled":
		return true
	}
	return strings.HasSuffix(strings.ToLower(name), ".exe") || strings.HasSuffix(strings.ToLower(name), ".sh") || strings.HasSuffix(strings.ToLower(name), ".bat")
}
