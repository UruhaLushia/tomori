package packcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/modrinth"
	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	Modrinth    = "modrinth"
	CurseForge  = "curseforge"
	PageSize    = 20
	modrinthAPI = "https://api.modrinth.com/v2"
	curseAPI    = "https://api.curseforge.com/v1"
)

type Client struct{ Fetcher *httpclient.Client }
type Filter struct {
	Source, Query, Category, GameVersion, Sort string
	Offset                                     int
}
type Category struct{ ID, Name string }
type Project struct {
	ID, Name, Description, Author, IconURL, Website string
	Categories                                      []string
	Downloads                                       int64
	Updated                                         time.Time
}
type Result struct {
	Projects      []Project
	Total, Offset int
}
type Version struct {
	Source, ProjectID                             string
	ID, Name, Number, URL, Filename, SHA1, SHA512 string
	GameVersions                                  []string
	Loaders                                       []string
	Size                                          int64
	Published                                     time.Time
}

func (v Version) OperationKey() string { return "pack:" + v.Source + ":" + v.ProjectID + ":" + v.ID }

func (c Client) Search(ctx context.Context, f Filter) (Result, error) {
	if f.Source == CurseForge {
		return c.curseSearch(ctx, f)
	}
	if f.Source != Modrinth {
		return Result{}, errors.New(locales.UnknownModpackSource())
	}
	facets := [][]string{{"project_type:modpack"}}
	if f.Category != "" {
		facets = append(facets, []string{"categories:" + f.Category})
	}
	if f.GameVersion != "" {
		facets = append(facets, []string{"versions:" + f.GameVersion})
	}
	raw, _ := json.Marshal(facets)
	index := map[string]string{"popular": "relevance", "downloads": "downloads", "updated": "updated", "newest": "newest"}[f.Sort]
	if index == "" {
		index = "relevance"
	}
	params := url.Values{"query": {f.Query}, "facets": {string(raw)}, "limit": {strconv.Itoa(PageSize)}, "offset": {strconv.Itoa(max(0, f.Offset))}, "index": {index}}
	var data struct {
		Hits []struct {
			modrinth.Project
			IconURL    string    `json:"icon_url"`
			Categories []string  `json:"display_categories"`
			Modified   time.Time `json:"date_modified"`
		}
		Total  int `json:"total_hits"`
		Offset int
	}
	if err := c.Fetcher.GetJSON(ctx, modrinthAPI+"/search?"+params.Encode(), &data); err != nil {
		return Result{}, err
	}
	r := Result{Total: data.Total, Offset: data.Offset}
	for _, p := range data.Hits {
		r.Projects = append(r.Projects, Project{ID: p.ID, Name: p.Title, Description: p.Description, Author: p.Author, IconURL: p.IconURL, Categories: p.Categories, Downloads: p.Downloads, Updated: p.Modified})
	}
	return r, nil
}

func (c Client) Categories(ctx context.Context, source string) ([]Category, error) {
	var result []Category
	if source == CurseForge {
		var data struct {
			Data []struct {
				ID      int
				Name    string
				ClassID int `json:"classId"`
			}
		}
		if err := c.Fetcher.GetJSON(ctx, curseAPI+"/categories?gameId=432&classId=4471", &data); err != nil {
			return nil, err
		}
		for _, v := range data.Data {
			if v.ClassID == 4471 {
				result = append(result, Category{strconv.Itoa(v.ID), v.Name})
			}
		}
		return result, nil
	}
	var data []struct {
		Name        string
		ProjectType string `json:"project_type"`
	}
	if err := c.Fetcher.GetJSON(ctx, modrinthAPI+"/tag/category", &data); err != nil {
		return nil, err
	}
	for _, v := range data {
		if v.ProjectType == "modpack" {
			result = append(result, Category{v.Name, v.Name})
		}
	}
	return result, nil
}

func (c Client) Versions(ctx context.Context, source, project, game string) ([]Version, error) {
	if source == CurseForge {
		return c.curseVersions(ctx, project, game)
	}
	versions, err := (modrinth.Client{Fetcher: c.Fetcher}).Versions(ctx, project, modrinth.Filter{GameVersion: game})
	if err != nil {
		return nil, err
	}
	var result []Version
	for _, v := range versions {
		if file, ok := modrinthVersion(v); ok {
			result = append(result, file)
		}
	}
	return result, nil
}

func modrinthVersion(v modrinth.Version) (Version, bool) {
	for _, primary := range []bool{true, false} {
		for _, f := range v.Files {
			if f.Primary == primary && strings.HasSuffix(strings.ToLower(f.Name), ".mrpack") {
				return Version{Source: Modrinth, ProjectID: v.ProjectID, ID: v.ID, Name: v.Name, Number: v.Number, URL: f.URL, Filename: f.Name, SHA1: f.Hashes["sha1"], SHA512: f.Hashes["sha512"], Size: f.Size, GameVersions: v.GameVersions, Loaders: v.Loaders, Published: v.Published}, true
			}
		}
	}
	return Version{}, false
}

// Resolve accepts an archive URL, a Modrinth pack/version page, or a CurseForge
// pack/file page. Project pages resolve to their latest downloadable release.
func (c Client) Resolve(ctx context.Context, raw string) (Version, error) {
	v, _, err := c.resolve(ctx, raw, false)
	return v, err
}

func (c Client) ResolveReference(ctx context.Context, raw string) (Version, []Version, error) {
	return c.resolve(ctx, raw, true)
}

func (c Client) resolve(ctx context.Context, raw string, reference bool) (Version, []Version, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil || u.Scheme != "https" && u.Scheme != "http" {
		return Version{}, nil, errors.New(locales.EnterAValidHttpOrHttpsUrl())
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	switch strings.ToLower(u.Hostname()) {
	case "modrinth.com", "www.modrinth.com":
		if len(parts) < 2 || parts[0] != "modpack" {
			return Version{}, nil, errors.New(locales.UseAModrinthModpackOrVersionPageUrl())
		}
		if len(parts) >= 4 && parts[2] == "version" {
			var v modrinth.Version
			if err := c.Fetcher.GetJSON(ctx, modrinthAPI+"/project/"+url.PathEscape(parts[1])+"/version/"+url.PathEscape(parts[3]), &v); err != nil {
				return Version{}, nil, err
			}
			if f, ok := modrinthVersion(v); ok {
				return f, nil, nil
			}
			return Version{}, nil, errors.New(locales.ThisVersionHasNoMrpackFile())
		}
		versions, err := c.Versions(ctx, Modrinth, parts[1], "")
		v, err := latest(versions, err)
		return v, versions, err
	case "curseforge.com", "www.curseforge.com":
		if len(parts) < 3 || parts[0] != "minecraft" || parts[1] != "modpacks" {
			return Version{}, nil, errors.New(locales.UseACurseForgeModpackOrFilePageUrl())
		}
		r, err := c.curseSearchSlug(ctx, parts[2])
		if err != nil {
			return Version{}, nil, err
		}
		if len(r.Projects) != 1 {
			return Version{}, nil, errors.New(locales.CurseForgeModpackNotFound())
		}
		if len(parts) >= 5 && (parts[3] == "files" || parts[3] == "download") {
			v, err := c.curseFile(ctx, r.Projects[0].ID, parts[4], reference)
			return v, nil, err
		}
		versions, err := c.Versions(ctx, CurseForge, r.Projects[0].ID, "")
		if reference && err == nil && len(versions) > 0 {
			return versions[0], versions, nil
		}
		v, err := latest(versions, err)
		return v, versions, err
	}
	u.Fragment = ""
	name := parts[len(parts)-1]
	if fsutil.ValidateID(name) != nil {
		name = "pack.zip"
	}
	return Version{URL: u.String(), Filename: name}, nil, nil
}

func latest(versions []Version, err error) (Version, error) {
	if err != nil {
		return Version{}, err
	}
	for _, v := range versions {
		if v.URL != "" {
			return v, nil
		}
	}
	return Version{}, errors.New(locales.NoDownloadableVersionDownloadInBrowserAndImport())
}
