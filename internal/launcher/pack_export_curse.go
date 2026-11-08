package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) exportCurseReferences(ctx context.Context, root *os.Root, include func(string, bool) bool, progress func(minecraft.Progress)) (map[string]exportReference, bool, error) {
	result := make(map[string]exportReference)
	projects := make(map[int64]string)
	var batch []resourcecatalog.Fingerprint
	files := make(map[string]exportReference)
	offline := false
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		query, cancel := context.WithTimeout(ctx, 20*time.Second)
		known, err := (resourcecatalog.Client{Fetcher: s.http, CacheDir: s.cacheDir, Lock: s.client.Lock}).IdentifyCurse(query, batch)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			offline = true
			return nil
		}
		for project, remote := range known {
			file, found := files[remote.SHA1]
			if !found || remote.Copies != 1 || file.Path != "mods/"+remote.Number {
				continue
			}
			file.project, _ = strconv.ParseInt(project, 10, 64)
			file.file, _ = strconv.ParseInt(remote.VersionID, 10, 64)
			if previous, exists := projects[file.project]; exists {
				delete(result, previous)
				projects[file.project] = ""
				continue
			}
			projects[file.project], result[file.Path] = file.Path, file
		}
		clear(files)
		batch = batch[:0]
		return nil
	}
	count := 0
	var total int64
	err := fsutil.WalkFiles(ctx, root, func(name string, dir bool) bool {
		top, _, _ := strings.Cut(filepath.ToSlash(name), "/")
		return top == "mods" && (include == nil || include(name, dir))
	}, func(name string, info os.FileInfo) error {
		if offline || filepath.Dir(name) != "mods" || !strings.EqualFold(filepath.Ext(name), ".jar") {
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
		file, err := fsutil.OpenRootRegular(root, name)
		if err != nil {
			release()
			return err
		}
		hash, err := resourcecatalog.FingerprintFile(ctx, file)
		after, statErr := file.Stat()
		closeErr := file.Close()
		release()
		if err := errors.Join(err, statErr, closeErr); err != nil {
			return err
		}
		if hash.Size != info.Size() || !sameExportFile(info, after) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		hash.Enabled = true
		batch = append(batch, hash)
		files[hash.SHA1] = exportReference{Path: filepath.ToSlash(name), Size: hash.Size, info: info}
		if progress != nil && count%25 == 0 {
			progress(minecraft.Progress{Stage: "export-identify", FilesDone: count, Done: total})
		}
		if len(batch) == 100 {
			return flush()
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	if !offline {
		err = flush()
	}
	return result, offline, err
}

func writeExportCurse(z *fsutil.ZipExporter, request PackExport, components minecraft.InstanceComponents, references map[string]exportReference) error {
	type loader struct {
		ID      string `json:"id"`
		Primary bool   `json:"primary"`
	}
	type file struct {
		ProjectID int64 `json:"projectID"`
		FileID    int64 `json:"fileID"`
		Required  bool  `json:"required"`
	}
	manifest := struct {
		Minecraft struct {
			Version string   `json:"version"`
			Loaders []loader `json:"modLoaders"`
		} `json:"minecraft"`
		Type      string `json:"manifestType"`
		Schema    int    `json:"manifestVersion"`
		Name      string `json:"name"`
		Version   string `json:"version"`
		Author    string `json:"author"`
		Files     []file `json:"files"`
		Overrides string `json:"overrides"`
	}{Type: "minecraftModpack", Schema: 1, Name: request.Name, Version: request.Version, Files: []file{}, Overrides: "overrides"}
	manifest.Minecraft.Version = components.Game
	manifest.Minecraft.Loaders = []loader{}
	if components.Loader != "" {
		manifest.Minecraft.Loaders = append(manifest.Minecraft.Loaders, loader{components.Loader + "-" + components.LoaderVersion, true})
	}
	for _, ref := range references {
		manifest.Files = append(manifest.Files, file{ref.project, ref.file, true})
	}
	slices.SortFunc(manifest.Files, func(a, b file) int {
		if a.ProjectID < b.ProjectID {
			return -1
		}
		if a.ProjectID > b.ProjectID {
			return 1
		}
		return 0
	})
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return z.Bytes("manifest.json", raw)
}
