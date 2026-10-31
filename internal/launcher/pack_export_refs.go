package launcher

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modrinth"
	"github.com/UruhaLushia/tomori/locales"
)

type exportReference struct {
	Path          string            `json:"path"`
	Hashes        map[string]string `json:"hashes"`
	Downloads     []string          `json:"downloads"`
	Size          int64             `json:"fileSize"`
	project, file int64
	info          os.FileInfo
}

func (s *Service) exportReferences(ctx context.Context, root *os.Root, include func(string, bool) bool, progress func(minecraft.Progress)) (map[string]exportReference, bool, error) {
	result := make(map[string]exportReference)
	var batch []exportReference
	var total int64
	offline := false
	buffer := make([]byte, 64<<10)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		var hashes []string
		for _, file := range batch {
			hashes = append(hashes, file.Hashes["sha1"])
		}
		queryCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		versions, err := (modrinth.Client{Fetcher: s.client}).Identify(queryCtx, hashes)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			offline = true
			batch = nil
			return nil
		}
		for _, file := range batch {
			for _, remote := range versions[file.Hashes["sha1"]].Files {
				if remote.Size == file.Size && remote.Hashes["sha1"] == file.Hashes["sha1"] && remote.Hashes["sha512"] == file.Hashes["sha512"] && exportURL(remote.URL) {
					file.Downloads = []string{remote.URL}
					result[file.Path] = file
					break
				}
			}
		}
		batch = nil
		return nil
	}
	count := 0
	err := fsutil.WalkFiles(ctx, root, func(path string, dir bool) bool {
		top, _, _ := strings.Cut(filepath.ToSlash(path), "/")
		if !slices.Contains([]string{"mods", "coremods", "resourcepacks", "texturepacks", "shaderpacks"}, top) {
			return false
		}
		return include == nil || include(path, dir)
	}, func(path string, info os.FileInfo) error {
		if offline {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(strings.TrimSuffix(path, ".disabled")))
		if ext != ".jar" && ext != ".zip" && ext != ".litemod" {
			return nil
		}
		count++
		total += info.Size()
		if count > 10000 || info.Size() > 4<<30 || info.Size() < 0 || total > 64<<30 {
			return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
		}
		release, err := s.client.FilePermit(ctx)
		if err != nil {
			return err
		}
		f, err := fsutil.OpenRootRegular(root, path)
		if err != nil {
			release()
			return err
		}
		h1, h512 := sha1.New(), sha512.New()
		n, err := io.CopyBuffer(io.MultiWriter(h1, h512), io.LimitReader(fsutil.ContextReader(ctx, f), info.Size()+1), buffer)
		after, statErr := f.Stat()
		_ = f.Close()
		release()
		if err != nil {
			return err
		}
		if statErr != nil {
			return statErr
		}
		if n != info.Size() || !sameExportFile(info, after) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		batch = append(batch, exportReference{Path: filepath.ToSlash(path), Size: info.Size(), Hashes: map[string]string{"sha1": hex.EncodeToString(h1.Sum(nil)), "sha512": hex.EncodeToString(h512.Sum(nil))}, info: info})
		if progress != nil && count%25 == 0 {
			progress(minecraft.Progress{Stage: "export-identify", FilesDone: count, Done: total})
		}
		if len(batch) >= 100 {
			return flush()
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if err := flush(); err != nil {
		return nil, false, err
	}
	return result, offline, nil
}

func exportURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Port() != "" || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	return slices.Contains([]string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com"}, u.Host)
}

func verifyExportReferences(ctx context.Context, root *os.Root, refs map[string]exportReference) error {
	for path, ref := range refs {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := root.Lstat(filepath.FromSlash(path))
		if err != nil {
			return err
		}
		if !sameExportFile(ref.info, info) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
	}
	return nil
}

func writeExportIndex(z *fsutil.ZipExporter, request PackExport, components minecraft.InstanceComponents, refs map[string]exportReference) error {
	dependencies := map[string]string{"minecraft": components.Game}
	if components.Loader != "" {
		id := components.Loader
		if id == "fabric" || id == "quilt" {
			id += "-loader"
		}
		dependencies[id] = components.LoaderVersion
	}
	files := make([]exportReference, 0, len(refs))
	for _, ref := range refs {
		files = append(files, ref)
	}
	slices.SortFunc(files, func(a, b exportReference) int { return strings.Compare(a.Path, b.Path) })
	index := struct {
		Format       int               `json:"formatVersion"`
		Game         string            `json:"game"`
		Version      string            `json:"versionId"`
		Name         string            `json:"name"`
		Summary      string            `json:"summary,omitempty"`
		Files        []exportReference `json:"files"`
		Dependencies map[string]string `json:"dependencies"`
	}{1, "minecraft", request.Version, request.Name, request.Description, files, dependencies}
	raw, err := json.Marshal(index)
	if err != nil {
		return err
	}
	if len(raw) > 16<<20 {
		return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
	}
	return z.Bytes("modrinth.index.json", raw)
}
