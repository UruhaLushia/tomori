package modpack

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/locales"
)

type MissingFile struct {
	Path, Name, SHA1, Website string
	Size                      int64
}
type MissingFilesError struct{ Files []MissingFile }

func (e *MissingFilesError) Error() string {
	return fmt.Sprintf(locales.FilesRequireManualDownload(), len(e.Files))
}

func Resolve(ctx context.Context, fetcher *download.Client, info Info, key string, optional bool) ([]File, error) {
	var files []File
	var ids []int64
	wanted := make(map[int64]File)
	for _, f := range info.Files {
		if f.Optional && !optional {
			continue
		}
		if f.FileID > 0 {
			if _, exists := wanted[f.FileID]; exists {
				return nil, fmt.Errorf(locales.DuplicateCurseForgeFileInModpack(), f.FileID)
			}
			ids = append(ids, f.FileID)
			wanted[f.FileID] = f
			continue
		}
		if f.Size < 0 || !validHash(f.SHA1, 20) || !validHash(f.SHA512, 64) {
			return nil, fmt.Errorf(locales.FileHasNoValidChecksum(), f.Path)
		}
		if len(f.URLs) == 0 {
			return nil, fmt.Errorf(locales.FileHasNoDownloadUrl(), f.Path)
		}
		for _, u := range f.URLs {
			if err := validURL(u, false); err != nil {
				return nil, err
			}
		}
		files = append(files, f)
	}
	if len(ids) == 0 {
		return files, nil
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New(locales.ThisModpackRequiresACurseForgeApiKeyEnterItInNetworkSettings())
	}
	type cfFile struct {
		ID, ModID             int64
		FileName, DownloadURL string
		FileLength            int64
		Hashes                []struct {
			Value string
			Algo  int
		}
	}
	var resolved []cfFile
	for start := 0; start < len(ids); start += 50 {
		var response struct{ Data []cfFile }
		if err := fetcher.JSONHeaders(ctx, http.MethodPost, "https://api.curseforge.com/v1/mods/files", map[string]any{"fileIds": ids[start:min(start+50, len(ids))]}, map[string]string{"x-api-key": key}, &response); err != nil {
			return nil, fmt.Errorf(locales.CurseForgeFileLookupFailed(), err)
		}
		resolved = append(resolved, response.Data...)
	}
	if len(resolved) != len(wanted) {
		return nil, errors.New(locales.SomeCurseForgeFilesWereRemovedOrAreUnavailable())
	}
	type projectInfo struct {
		ID      int64
		ClassID int
		Links   struct{ WebsiteURL string }
	}
	projects := make(map[int64]projectInfo)
	var projectIDs []int64
	for _, f := range wanted {
		if !slices.Contains(projectIDs, f.ProjectID) {
			projectIDs = append(projectIDs, f.ProjectID)
		}
	}
	for start := 0; start < len(projectIDs); start += 50 {
		var response struct{ Data []projectInfo }
		if err := fetcher.JSONHeaders(ctx, http.MethodPost, "https://api.curseforge.com/v1/mods", map[string]any{"modIds": projectIDs[start:min(start+50, len(projectIDs))]}, map[string]string{"x-api-key": key}, &response); err != nil {
			return nil, err
		}
		for _, item := range response.Data {
			projects[item.ID] = item
		}
	}
	seen := make(map[int64]bool)
	for _, item := range resolved {
		f, ok := wanted[item.ID]
		if !ok || item.ModID != f.ProjectID || seen[item.ID] {
			return nil, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
		}
		seen[item.ID] = true
		project, ok := projects[item.ModID]
		if !ok {
			return nil, fmt.Errorf(locales.CurseForgeProjectRemovedOrUnavailable(), item.ModID)
		}
		class := project.ClassID
		folder := "mods"
		switch class {
		case 12:
			folder = "resourcepacks"
		case 6552:
			folder = "shaderpacks"
		case 6:
		default:
			return nil, fmt.Errorf(locales.UnsupportedCurseForgeContentType(), class)
		}
		if item.FileName == "" || item.FileName == "." || item.FileName == ".." || path.Base(item.FileName) != item.FileName || strings.ContainsAny(item.FileName, "\\:") {
			return nil, fmt.Errorf(locales.InvalidFilename(), item.FileName)
		}
		f.Path = folder + "/" + item.FileName
		f.Size = item.FileLength
		if f.Size < 0 {
			return nil, fmt.Errorf(locales.InvalidFileSize(), f.Path)
		}
		if u, err := url.Parse(project.Links.WebsiteURL); err == nil && u.Scheme == "https" && u.Host == "www.curseforge.com" {
			f.Website = strings.TrimRight(project.Links.WebsiteURL, "/") + fmt.Sprintf("/download/%d", item.ID)
		}
		for _, h := range item.Hashes {
			if h.Algo == 1 {
				f.SHA1 = h.Value
			}
		}
		if !validHash(f.SHA1, 20) {
			return nil, fmt.Errorf(locales.FileHasNoSha1(), f.Path)
		}
		if item.DownloadURL != "" {
			if err := validURL(item.DownloadURL, true); err != nil {
				return nil, err
			}
			f.URLs = []string{item.DownloadURL}
		}
		files = append(files, f)
	}
	return files, nil
}

func validHash(value string, bytes int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == bytes
}
func validURL(raw string, cf bool) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" {
		return errors.New(locales.InvalidDownloadUrl())
	}
	hosts := []string{"cdn.modrinth.com", "github.com", "raw.githubusercontent.com", "gitlab.com"}
	if cf {
		hosts = []string{"edge.forgecdn.net", "media.forgecdn.net", "mediafilez.forgecdn.net", "files.forgecdn.net"}
	}
	if !slices.Contains(hosts, strings.ToLower(u.Hostname())) || u.Port() != "" {
		return fmt.Errorf(locales.UnsupportedModpackSource(), u.Hostname())
	}
	return nil
}
