package packcatalog

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type curseProject struct {
	ID            int
	Name, Summary string
	DownloadCount float64   `json:"downloadCount"`
	DateModified  time.Time `json:"dateModified"`
	Logo          struct{ URL string }
	Links         struct {
		WebsiteURL string `json:"websiteUrl"`
	}
	Authors    []struct{ Name string }
	Categories []struct{ Name string }
}
type curseFile struct {
	ID           int
	ModID        int       `json:"modId"`
	DisplayName  string    `json:"displayName"`
	FileName     string    `json:"fileName"`
	DownloadURL  string    `json:"downloadUrl"`
	FileLength   int64     `json:"fileLength"`
	FileDate     time.Time `json:"fileDate"`
	GameVersions []string  `json:"gameVersions"`
	Hashes       []struct {
		Value string
		Algo  int
	}
}

func (c Client) curseSearch(ctx context.Context, f Filter) (Result, error) {
	sort := map[string]string{"popular": "2", "downloads": "6", "updated": "3", "newest": "11"}[f.Sort]
	if sort == "" {
		sort = "2"
	}
	p := url.Values{"gameId": {"432"}, "classId": {"4471"}, "searchFilter": {f.Query}, "sortField": {sort}, "sortOrder": {"desc"}, "pageSize": {strconv.Itoa(PageSize)}, "index": {strconv.Itoa(max(0, min(f.Offset, 9980)))}}
	if f.Category != "" {
		p.Set("categoryId", f.Category)
	}
	if f.GameVersion != "" {
		p.Set("gameVersion", f.GameVersion)
	}
	return c.curseProjects(ctx, p)
}
func (c Client) curseSearchSlug(ctx context.Context, slug string) (Result, error) {
	return c.curseProjects(ctx, url.Values{"gameId": {"432"}, "classId": {"4471"}, "slug": {slug}, "pageSize": {"1"}})
}
func (c Client) curseProjects(ctx context.Context, p url.Values) (Result, error) {
	var data struct {
		Data       []curseProject
		Pagination struct {
			TotalCount int `json:"totalCount"`
			Index      int
		}
	}
	if err := c.Fetcher.GetJSON(ctx, curseAPI+"/mods/search?"+p.Encode(), &data); err != nil {
		return Result{}, err
	}
	r := Result{Total: min(data.Pagination.TotalCount, 10000), Offset: data.Pagination.Index}
	for _, v := range data.Data {
		p := Project{ID: strconv.Itoa(v.ID), Name: v.Name, Description: v.Summary, Downloads: int64(v.DownloadCount), Updated: v.DateModified, IconURL: v.Logo.URL}
		var authors []string
		for _, a := range v.Authors {
			authors = append(authors, a.Name)
		}
		p.Author = strings.Join(authors, ", ")
		p.Website = v.Links.WebsiteURL
		for _, category := range v.Categories {
			p.Categories = append(p.Categories, category.Name)
		}
		r.Projects = append(r.Projects, p)
	}
	return r, nil
}
func (c Client) curseVersions(ctx context.Context, project, game string) ([]Version, error) {
	if _, err := strconv.ParseUint(project, 10, 64); err != nil {
		return nil, errors.New(locales.InvalidCurseForgeProjectId())
	}
	var result []Version
	for offset := 0; offset < 10000; offset += 50 {
		p := url.Values{"pageSize": {"50"}, "index": {strconv.Itoa(offset)}}
		if game != "" {
			p.Set("gameVersion", game)
		}
		var data struct {
			Data       []curseFile
			Pagination struct {
				TotalCount int `json:"totalCount"`
			}
		}
		if err := c.Fetcher.GetJSON(ctx, curseAPI+"/mods/"+project+"/files?"+p.Encode(), &data); err != nil {
			return nil, err
		}
		for _, f := range data.Data {
			if strconv.Itoa(f.ModID) != project || f.ID <= 0 {
				return nil, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
			}
			result = append(result, convertCurseFile(f))
		}
		if len(data.Data) < 50 || offset+len(data.Data) >= data.Pagination.TotalCount {
			break
		}
	}
	slices.SortStableFunc(result, func(a, b Version) int { return b.Published.Compare(a.Published) })
	return result, nil
}
func (c Client) curseFile(ctx context.Context, project, file string, reference bool) (Version, error) {
	if _, err := strconv.ParseUint(file, 10, 64); err != nil {
		return Version{}, errors.New(locales.InvalidCurseforgeFileIdAlt())
	}
	var data struct{ Data curseFile }
	if err := c.Fetcher.GetJSON(ctx, curseAPI+"/mods/"+project+"/files/"+file, &data); err != nil {
		return Version{}, err
	}
	if strconv.Itoa(data.Data.ModID) != project || strconv.Itoa(data.Data.ID) != file || data.Data.ID <= 0 {
		return Version{}, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
	}
	v := convertCurseFile(data.Data)
	if v.URL == "" && !reference {
		return Version{}, errors.New(locales.DownloadThisFileInBrowserThenImportIt())
	}
	return v, nil
}
func convertCurseFile(f curseFile) Version {
	v := Version{Source: CurseForge, ProjectID: strconv.Itoa(f.ModID), ID: strconv.Itoa(f.ID), Name: f.DisplayName, Number: f.DisplayName, Filename: f.FileName, URL: f.DownloadURL, Size: f.FileLength, Published: f.FileDate, GameVersions: f.GameVersions}
	for _, h := range f.Hashes {
		if h.Algo == 1 {
			v.SHA1 = h.Value
		}
	}
	for _, tag := range f.GameVersions {
		switch strings.ToLower(tag) {
		case "forge", "neoforge", "fabric", "quilt":
			v.Loaders = append(v.Loaders, strings.ToLower(tag))
		}
	}
	return v
}
