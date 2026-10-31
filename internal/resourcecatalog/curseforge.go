package resourcecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

const curseAPI = "https://api.curseforge.com/v1"

type curseProject struct {
	ID                  int
	ClassID             int `json:"classId"`
	Name, Summary, Slug string
	DownloadCount       float64 `json:"downloadCount"`
	AllowDistribution   *bool   `json:"allowModDistribution"`
	Links               struct {
		Website string `json:"websiteUrl"`
	}
	Authors    []struct{ Name string }
	Categories []struct {
		Name string
	}
	Logo struct {
		ThumbnailURL string `json:"thumbnailUrl"`
	}
}
type curseFile struct {
	ID           int
	ProjectID    int       `json:"modId"`
	Name         string    `json:"displayName"`
	Filename     string    `json:"fileName"`
	URL          string    `json:"downloadUrl"`
	Size         int64     `json:"fileLength"`
	Published    time.Time `json:"fileDate"`
	Available    bool      `json:"isAvailable"`
	Release      int       `json:"releaseType"`
	GameVersions []string  `json:"gameVersions"`
	Hashes       []struct {
		Value string
		Algo  int
	}
	Dependencies []struct {
		ProjectID int `json:"modId"`
		Relation  int `json:"relationType"`
	}
}

func class(kind resource.Kind) int {
	switch kind {
	case resource.Mod:
		return 6
	case resource.ResourcePack:
		return 12
	case resource.ShaderPack:
		return 6552
	case resource.DataPack:
		return 6945
	case resource.World:
		return 17
	}
	return 0
}
func classKind(id int) resource.Kind {
	switch id {
	case 6:
		return resource.Mod
	case 12:
		return resource.ResourcePack
	case 6552:
		return resource.ShaderPack
	case 6945:
		return resource.DataPack
	case 17:
		return resource.World
	}
	return ""
}
func curseLoader(loader string) int {
	switch loader {
	case "forge":
		return 1
	case "liteloader":
		return 3
	case "fabric":
		return 4
	case "quilt":
		return 5
	case "neoforge":
		return 6
	}
	return 0
}
func validID(id string) bool {
	value, err := strconv.ParseUint(id, 10, 32)
	return err == nil && value > 0
}

func (c Client) curseSearch(ctx context.Context, query string, filter Filter, offset int) (SearchResult, error) {
	p := url.Values{"gameId": {"432"}, "classId": {strconv.Itoa(class(filter.Kind))}, "searchFilter": {query}, "pageSize": {strconv.Itoa(PageSize)}, "index": {strconv.Itoa(max(0, min(offset, 10000-PageSize)))}, "sortField": {"2"}, "sortOrder": {"desc"}}
	if filter.GameVersion != "" {
		p.Set("gameVersion", filter.GameVersion)
	}
	for _, category := range filter.Categories {
		if strings.HasPrefix(category, "cf:") && validID(strings.TrimPrefix(category, "cf:")) {
			p.Set("categoryId", strings.TrimPrefix(category, "cf:"))
			break
		}
	}
	if filter.Kind == resource.Mod && len(filter.Loaders) > 0 {
		var loaders []int
		for _, loader := range filter.Loaders {
			if id := curseLoader(loader); id != 0 && !slices.Contains(loaders, id) {
				loaders = append(loaders, id)
			}
		}
		if len(loaders) == 0 {
			return SearchResult{}, nil
		}
		raw, _ := json.Marshal(loaders)
		p.Set("modLoaderTypes", string(raw))
	}
	var data struct {
		Data       []curseProject
		Pagination struct {
			Total int `json:"totalCount"`
			Index int
		}
	}
	if err := c.Fetcher.GetJSON(ctx, curseAPI+"/mods/search?"+p.Encode(), &data); err != nil {
		return SearchResult{}, err
	}
	r := SearchResult{Total: min(data.Pagination.Total, 10000), Offset: data.Pagination.Index}
	for _, v := range data.Data {
		if v.ID <= 0 || classKind(v.ClassID) != filter.Kind {
			continue
		}
		var authors []string
		for _, a := range v.Authors {
			authors = append(authors, a.Name)
		}
		var categories []string
		for _, category := range v.Categories {
			if category.Name != "" {
				categories = append(categories, category.Name)
			}
		}
		r.Hits = append(r.Hits, Project{Source: CurseForge, ID: strconv.Itoa(v.ID), Slug: v.Slug, Title: v.Name, Description: v.Summary, Author: strings.Join(authors, ", "), IconURL: v.Logo.ThumbnailURL, Categories: categories, Downloads: int64(v.DownloadCount), Website: v.website()})
	}
	return r, nil
}

func (p curseProject) website() string {
	u, err := url.Parse(p.Links.Website)
	if err == nil && u.Scheme == "https" && u.Host == "www.curseforge.com" && u.User == nil {
		return strings.TrimRight(u.String(), "/")
	}
	return ""
}
func (f curseFile) version(website string) Version {
	if website != "" {
		website = strings.TrimRight(website, "/") + "/files/" + strconv.Itoa(f.ID)
	}
	dependencies := make([]string, 0, len(f.Dependencies))
	for _, dependency := range f.Dependencies {
		if dependency.ProjectID > 0 {
			dependencies = append(dependencies, strconv.Itoa(dependency.ProjectID))
		}
	}
	return Version{
		Source: CurseForge, ID: strconv.Itoa(f.ID), ProjectID: strconv.Itoa(f.ProjectID), Name: f.Name,
		Number: f.Filename, Channel: map[int]string{1: "release", 2: "beta", 3: "alpha"}[f.Release],
		Published: f.Published, Downloadable: f.Available && f.URL != "", GameVersions: slices.Clone(f.GameVersions), Dependencies: dependencies, Size: f.Size,
		Website: website,
	}
}
func (f curseFile) compatible(filter Filter) bool {
	if !f.Available {
		return false
	}
	if filter.GameVersion != "" && !slices.Contains(f.GameVersions, filter.GameVersion) {
		return false
	}
	if filter.Kind != resource.Mod {
		return filter.Kind.Accepts(f.Filename, false)
	}
	if len(filter.Loaders) == 0 {
		return true
	}
	for _, tag := range f.GameVersions {
		for _, loader := range filter.Loaders {
			if curseLoader(loader) != 0 && strings.EqualFold(tag, loader) {
				return true
			}
		}
	}
	return false
}

func (c Client) curseFiles(ctx context.Context, project string, filter Filter, visit func(curseFile)) error {
	if !validID(project) {
		return errors.New(locales.InvalidCurseForgeProjectId())
	}
	for offset := 0; offset < 10000; offset += 50 {
		p := url.Values{"pageSize": {"50"}, "index": {strconv.Itoa(offset)}}
		if filter.GameVersion != "" {
			p.Set("gameVersion", filter.GameVersion)
		}
		if filter.Kind == resource.Mod && len(filter.Loaders) == 1 && curseLoader(filter.Loaders[0]) != 0 {
			p.Set("modLoaderType", strconv.Itoa(curseLoader(filter.Loaders[0])))
		}
		var data struct {
			Data       []curseFile
			Pagination struct {
				Total int `json:"totalCount"`
			}
		}
		body, err := c.Fetcher.GetLimited(ctx, curseAPI+"/mods/"+project+"/files?"+p.Encode(), 8<<20)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(body, &data); err != nil {
			return err
		}
		for _, f := range data.Data {
			if strconv.Itoa(f.ProjectID) == project && f.compatible(filter) {
				visit(f)
			}
		}
		if len(data.Data) < 50 || offset+len(data.Data) >= data.Pagination.Total {
			break
		}
	}
	return nil
}

func (c Client) curseProject(ctx context.Context, id string) (curseProject, error) {
	if !validID(id) {
		return curseProject{}, errors.New(locales.InvalidCurseForgeProjectId())
	}
	var response struct{ Data curseProject }
	if err := c.Fetcher.GetJSONLimited(ctx, curseAPI+"/mods/"+id, &response, 2<<20); err != nil {
		return curseProject{}, err
	}
	if strconv.Itoa(response.Data.ID) != id || !classKind(response.Data.ClassID).Valid() {
		return curseProject{}, errors.New(locales.CurseForgeReturnedAnUnexpectedProject())
	}
	return response.Data, nil
}

func (c Client) curseProjects(ctx context.Context, ids []string) (map[string]curseProject, error) {
	result := make(map[string]curseProject, len(ids))
	if len(ids) == 1 {
		p, err := c.curseProject(ctx, ids[0])
		if err != nil {
			return nil, err
		}
		result[ids[0]] = p
		return result, nil
	}
	for start := 0; start < len(ids); start += 50 {
		batch := ids[start:min(start+50, len(ids))]
		values := make([]int, 0, len(batch))
		for _, id := range batch {
			if !validID(id) {
				return nil, errors.New(locales.InvalidCurseForgeProjectId())
			}
			v, _ := strconv.Atoi(id)
			values = append(values, v)
		}
		var response struct{ Data []curseProject }
		if err := c.Fetcher.PostJSONLimited(ctx, curseAPI+"/mods", map[string]any{"modIds": values}, &response, 8<<20); err != nil {
			return nil, err
		}
		seen := make(map[string]bool, len(batch))
		for _, p := range response.Data {
			id := strconv.Itoa(p.ID)
			if !slices.Contains(batch, id) || seen[id] || !classKind(p.ClassID).Valid() {
				return nil, errors.New(locales.CurseForgeReturnedAnUnexpectedProject())
			}
			seen[id] = true
			result[id] = p
		}
		if len(seen) != len(batch) {
			return nil, errors.New(locales.CurseForgeReturnedAnUnexpectedProject())
		}
	}
	return result, nil
}
