package modrinth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"time"

	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

const baseURL = "https://api.modrinth.com/v2"

type Client struct{ Fetcher *httpclient.Client }

type Filter struct {
	Kind        resource.Kind
	GameVersion string
	Loaders     []string
	Categories  []string
}

type Project struct {
	ID          string   `json:"project_id"`
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Author      string   `json:"author"`
	IconURL     string   `json:"icon_url"`
	Categories  []string `json:"categories"`
	Downloads   int64    `json:"downloads"`
}

type SearchResult struct {
	Hits   []Project `json:"hits"`
	Total  int       `json:"total_hits"`
	Offset int       `json:"offset"`
}

type Version struct {
	ID           string       `json:"id"`
	ProjectID    string       `json:"project_id"`
	Name         string       `json:"name"`
	Number       string       `json:"version_number"`
	Channel      string       `json:"version_type"`
	Published    time.Time    `json:"date_published"`
	GameVersions []string     `json:"game_versions"`
	Loaders      []string     `json:"loaders"`
	Files        []File       `json:"files"`
	Dependencies []Dependency `json:"dependencies"`
}

type File struct {
	Name    string            `json:"filename"`
	URL     string            `json:"url"`
	Size    int64             `json:"size"`
	Primary bool              `json:"primary"`
	Hashes  map[string]string `json:"hashes"`
}

type Dependency struct {
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	Type      string `json:"dependency_type"`
}

func (c Client) Search(ctx context.Context, query string, filter Filter, offset int) (SearchResult, error) {
	if !filter.Kind.Valid() {
		return SearchResult{}, errors.New(locales.InvalidResourceType())
	}
	facets := [][]string{{"project_type:" + filter.Kind.ProjectType()}}
	if filter.GameVersion != "" {
		facets = append(facets, []string{"versions:" + filter.GameVersion})
	}
	if filter.Kind == resource.DataPack {
		facets = append(facets, []string{"categories:datapack"})
	}
	if len(filter.Categories) > 0 {
		categories := make([]string, 0, len(filter.Categories))
		for _, category := range filter.Categories {
			if category != "" {
				categories = append(categories, "categories:"+category)
			}
		}
		if len(categories) > 0 {
			facets = append(facets, categories)
		}
	}
	if filter.Kind == resource.Mod && len(filter.Loaders) > 0 {
		var loaders []string
		for _, loader := range filter.Loaders {
			loaders = append(loaders, "categories:"+loader)
		}
		facets = append(facets, loaders)
	}
	raw, err := json.Marshal(facets)
	if err != nil {
		return SearchResult{}, err
	}
	params := url.Values{"query": {query}, "facets": {string(raw)}, "limit": {"20"}, "offset": {strconv.Itoa(max(0, offset))}, "index": {"relevance"}}
	var result SearchResult
	err = c.Fetcher.GetJSON(ctx, baseURL+"/search?"+params.Encode(), &result)
	return result, err
}

func (c Client) Versions(ctx context.Context, project string, filter Filter) ([]Version, error) {
	params := url.Values{"include_changelog": {"false"}}
	if filter.GameVersion != "" {
		raw, _ := json.Marshal([]string{filter.GameVersion})
		params.Set("game_versions", string(raw))
	}
	if filter.Kind == resource.Mod && len(filter.Loaders) > 0 {
		raw, _ := json.Marshal(filter.Loaders)
		params.Set("loaders", string(raw))
	}
	if filter.Kind == resource.DataPack {
		params.Set("loaders", `["datapack"]`)
	}
	var versions []Version
	if err := c.Fetcher.GetJSONLimited(ctx, baseURL+"/project/"+url.PathEscape(project)+"/version?"+params.Encode(), &versions, 8<<20); err != nil {
		return nil, err
	}
	slices.SortStableFunc(versions, func(a, b Version) int { return b.Published.Compare(a.Published) })
	return versions, nil
}

func (c Client) Version(ctx context.Context, id string) (Version, error) {
	var version Version
	err := c.Fetcher.GetJSONLimited(ctx, baseURL+"/version/"+url.PathEscape(id), &version, 8<<20)
	if err == nil && (version.ID != id || version.ProjectID == "") {
		err = errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
	}
	return version, err
}

func (c Client) VersionsByID(ctx context.Context, ids []string) ([]Version, error) {
	result := make([]Version, 0, len(ids))
	for start := 0; start < len(ids); start += 50 {
		batch := ids[start:min(start+50, len(ids))]
		raw, err := json.Marshal(batch)
		if err != nil {
			return nil, err
		}
		body, err := c.Fetcher.GetLimited(ctx, baseURL+"/versions?"+url.Values{"ids": {string(raw)}}.Encode(), 8<<20)
		if err != nil {
			return nil, err
		}
		var versions []Version
		if err := json.Unmarshal(body, &versions); err != nil {
			return nil, err
		}
		byID := make(map[string]Version, len(versions))
		for _, version := range versions {
			if version.ProjectID == "" || !slices.Contains(batch, version.ID) {
				return nil, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
			}
			if _, exists := byID[version.ID]; exists {
				return nil, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
			}
			byID[version.ID] = version
		}
		for _, id := range batch {
			version, found := byID[id]
			if !found {
				return nil, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
			}
			result = append(result, version)
		}
	}
	return result, nil
}

func (c Client) Identify(ctx context.Context, hashes []string) (map[string]Version, error) {
	result := make(map[string]Version)
	for len(hashes) > 0 {
		n := min(200, len(hashes))
		var batch map[string]Version
		err := c.Fetcher.PostJSONLimited(ctx, baseURL+"/version_files", map[string]any{"hashes": hashes[:n], "algorithm": "sha1"}, &batch, 8<<20)
		if err != nil {
			return nil, err
		}
		for hash, version := range batch {
			if slices.Contains(hashes[:n], hash) {
				result[hash] = version
			}
		}
		hashes = hashes[n:]
	}
	return result, nil
}

func (c Client) LatestReleases(ctx context.Context, hashes []string, filter Filter) (map[string]Version, error) {
	result := make(map[string]Version, len(hashes))
	for start := 0; start < len(hashes); start += 50 {
		batch := hashes[start:min(start+50, len(hashes))]
		body := map[string]any{"hashes": batch, "algorithm": "sha1", "game_versions": []string{filter.GameVersion}, "version_types": []string{"release"}, "loaders": []string{}}
		switch filter.Kind {
		case resource.Mod:
			body["loaders"] = filter.Loaders
		case resource.DataPack:
			body["loaders"] = []string{"datapack"}
		}
		var versions map[string]Version
		if err := c.Fetcher.PostJSONLimited(ctx, baseURL+"/version_files/update", body, &versions, 8<<20); err != nil {
			return nil, err
		}
		for hash, version := range versions {
			if slices.Contains(batch, hash) && version.Channel == "release" && version.Compatible(filter) {
				result[hash] = version
			}
		}
	}
	return result, nil
}

func (v Version) PrimaryFile(kind resource.Kind) (File, error) {
	for _, primary := range []bool{true, false} {
		for _, file := range v.Files {
			if file.Primary == primary && kind.Accepts(file.Name, false) {
				return file, nil
			}
		}
	}
	return File{}, fmt.Errorf(locales.NoInstallableFilesFor(), v.Name)
}

func (v Version) Compatible(filter Filter) bool {
	if filter.GameVersion == "" || !slices.Contains(v.GameVersions, filter.GameVersion) {
		return false
	}
	if filter.Kind == resource.DataPack {
		return slices.Contains(v.Loaders, "datapack")
	}
	if filter.Kind != resource.Mod {
		return true
	}
	for _, loader := range filter.Loaders {
		if slices.Contains(v.Loaders, loader) {
			return true
		}
	}
	return false
}
