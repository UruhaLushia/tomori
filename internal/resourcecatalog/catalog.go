package resourcecatalog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/modrinth"
	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/i18n"
)

type Source string

const (
	Modrinth   Source = "modrinth"
	CurseForge Source = "curseforge"
	PageSize          = 20
)

func (s Source) Valid() bool { return s == Modrinth || s == CurseForge }
func (s Source) Label() string {
	if s == CurseForge {
		return "CurseForge"
	}
	return "Modrinth"
}

type Filter = modrinth.Filter

// Empty fields follow the installation target; AnyCompatibility removes that constraint.
const AnyCompatibility = "*"

type CompatibilityFilter struct {
	GameVersion string
	Loader      string
}

func (f CompatibilityFilter) Apply(filter Filter) Filter {
	switch f.GameVersion {
	case "":
	case AnyCompatibility:
		filter.GameVersion = ""
	default:
		filter.GameVersion = f.GameVersion
	}
	switch f.Loader {
	case "":
	case AnyCompatibility:
		filter.Loaders = nil
	default:
		filter.Loaders = []string{f.Loader}
	}
	return filter
}

type Client struct {
	Fetcher  *httpclient.Client
	CacheDir string
	Lock     func(context.Context, string, string) (func(), error)
}
type Project struct {
	Source                                  Source
	ID, Title, Description, Author, Website string
	Slug, TranslatedName                    string
	TranslatedDescription                   string
	IconURL                                 string
	Categories                              []string
	Downloads                               int64
}
type SearchResult struct {
	Hits          []Project
	Total, Offset int
	ResolvedQuery string
	Suggestions   []NameSuggestion
	NameWarning   bool
}
type Version struct {
	Source                                        Source
	ID, ProjectID, Name, Number, Channel, Website string
	Published                                     time.Time
	Downloadable                                  bool
	GameVersions                                  []string
	Loaders                                       []string
	Dependencies                                  []string
	Size                                          int64
}
type InstallFile struct {
	Version         Version
	Kind            resource.Kind
	Name, URL, SHA1 string
	Size            int64
	Disabled        bool
	Manual          bool
}

type ResourceUpdate struct {
	Size    int64
	Entry   resource.Entry
	SHA1    string
	Origin  resource.Origin
	Version Version
}

type UpdateCheck struct {
	Updates                     []ResourceUpdate
	Checked, Unknown, Conflicts int
}
type Installed struct {
	VersionID string
	Enabled   bool
	SHA1      string
	Number    string
	Copies    int
	file      *curseFile
}
type MissingFile struct {
	Name, Website, SHA1 string
	Size                int64
	Kind                resource.Kind
}
type MissingFilesError struct{ Files []MissingFile }

func (e *MissingFilesError) Error() string {
	return fmt.Sprintf(locales.FilesRequireManualDownload(), len(e.Files))
}

func (c Client) Search(ctx context.Context, source Source, query string, filter Filter, offset int) (SearchResult, error) {
	if !source.Valid() || !filter.Kind.Valid() {
		return SearchResult{}, errors.New(locales.InvalidResourceSourceOrType())
	}
	query = strings.TrimSpace(query)
	resolved := query
	var suggestions []NameSuggestion
	chinese := hasChinese(query)
	var namesErr error
	if filter.Kind == resource.Mod {
		f, err := c.names(ctx, chinese || query != "" && len(query) <= 16)
		namesErr = err
		if f != nil {
			matches, lookupErr := lookupNames(ctx, f, source, query)
			if lookupErr != nil && ctx.Err() == nil {
				repaired, repairErr := c.cachedNames(ctx, true, f)
				_ = f.Close()
				f = repaired
				if repairErr == nil && f != nil {
					_, _ = f.Seek(0, 0)
					matches, lookupErr = lookupNames(ctx, f, source, query)
				}
			}
			if f != nil {
				defer f.Close()
			}
			if lookupErr != nil {
				namesErr = lookupErr
			} else if len(matches) > 0 {
				if len(matches) == 1 || matches[0].score < matches[1].score {
					resolved = matches[0].Query
				} else {
					for _, match := range matches {
						suggestions = append(suggestions, match.NameSuggestion)
					}
				}
			}
			result, searchErr := c.search(ctx, source, resolved, filter, offset)
			if searchErr == nil && result.Total == 0 && resolved != query {
				result, searchErr = c.search(ctx, source, query, filter, offset)
				resolved = query
			}
			if namesErr == nil && f != nil {
				if err := decorateNames(ctx, f, source, result.Hits); err != nil {
					namesErr = err
				}
			}
			if resolved != query {
				result.ResolvedQuery = resolved
			}
			result.Suggestions, result.NameWarning = suggestions, chinese && namesErr != nil
			if ctx.Err() != nil {
				return SearchResult{}, ctx.Err()
			}
			return result, searchErr
		}
	}
	result, err := c.search(ctx, source, query, filter, offset)
	if err == nil && namesErr == nil {
		if f, decorateErr := c.names(ctx, false); decorateErr == nil && f != nil {
			if decorateErr = decorateNames(ctx, f, source, result.Hits); decorateErr != nil {
				namesErr = decorateErr
			}
			_ = f.Close()
		}
	}
	result.NameWarning = chinese && filter.Kind == resource.Mod && namesErr != nil
	return result, err
}

func (c Client) search(ctx context.Context, source Source, query string, filter Filter, offset int) (SearchResult, error) {
	if source == CurseForge {
		return c.curseSearch(ctx, query, filter, offset)
	}
	r, err := (modrinth.Client{Fetcher: c.Fetcher}).Search(ctx, query, filter, offset)
	result := SearchResult{Total: r.Total, Offset: r.Offset}
	for _, p := range r.Hits {
		result.Hits = append(result.Hits, Project{Source: Modrinth, ID: p.ID, Slug: p.Slug, Title: p.Title, Description: p.Description, Author: p.Author, IconURL: p.IconURL, Categories: slices.Clone(p.Categories), Downloads: p.Downloads, Website: "https://modrinth.com/" + filter.Kind.ProjectType() + "/" + url.PathEscape(p.ID)})
	}
	return result, err
}

func (p Project) DisplayName() string {
	if !strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") && p.TranslatedName != "" && p.TranslatedName != p.Title {
		return p.TranslatedName + " · " + p.Title
	}
	return p.Title
}

func (p Project) DisplayDescription() string {
	if strings.HasPrefix(strings.ToLower(i18n.Locale()), "zh") && p.TranslatedDescription != "" {
		return p.TranslatedDescription
	}
	return p.Description
}

func (c Client) Versions(ctx context.Context, project Project, filter Filter) ([]Version, error) {
	if !project.Source.Valid() || !filter.Kind.Valid() {
		return nil, errors.New(locales.InvalidResourceSourceOrType())
	}
	if project.Source == CurseForge {
		metadata, err := c.curseProject(ctx, project.ID)
		if err != nil {
			return nil, err
		}
		if classKind(metadata.ClassID) != filter.Kind {
			return nil, errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
		var result []Version
		err = c.curseFiles(ctx, project.ID, filter, func(file curseFile) {
			v := file.version(metadata.website())
			if metadata.AllowDistribution != nil && !*metadata.AllowDistribution {
				v.Downloadable = false
			}
			result = append(result, v)
		})
		if err != nil {
			return nil, err
		}
		slices.SortStableFunc(result, func(a, b Version) int { return b.Published.Compare(a.Published) })
		return result, nil
	}
	versions, err := (modrinth.Client{Fetcher: c.Fetcher}).Versions(ctx, project.ID, filter)
	if err != nil {
		return nil, err
	}
	var result []Version
	for _, v := range versions {
		// Browsing may leave either compatibility constraint unset.
		if filter.GameVersion != "" && !slices.Contains(v.GameVersions, filter.GameVersion) {
			continue
		}
		if filter.Kind == resource.Mod && len(filter.Loaders) > 0 && !slices.ContainsFunc(v.Loaders, func(loader string) bool { return slices.Contains(filter.Loaders, loader) }) {
			continue
		}
		if filter.Kind == resource.DataPack && !slices.Contains(v.Loaders, "datapack") {
			continue
		}
		if _, err := v.PrimaryFile(filter.Kind); err == nil {
			result = append(result, fromModrinth(v))
		}
	}
	return result, nil
}

func fromModrinth(v modrinth.Version) Version {
	var size int64
	for _, file := range v.Files {
		if file.Primary {
			size = file.Size
			break
		}
		if size == 0 {
			size = file.Size
		}
	}
	dependencies := make([]string, 0, len(v.Dependencies))
	for _, dependency := range v.Dependencies {
		if dependency.ProjectID != "" {
			dependencies = append(dependencies, dependency.ProjectID)
		}
	}
	return Version{
		Source: Modrinth, ID: v.ID, ProjectID: v.ProjectID, Name: v.Name, Number: v.Number,
		Channel: v.Channel, Published: v.Published, Downloadable: true,
		GameVersions: slices.Clone(v.GameVersions), Loaders: slices.Clone(v.Loaders), Dependencies: dependencies, Size: size,
		Website: "https://modrinth.com/" + "project/" + url.PathEscape(v.ProjectID) + "/version/" + url.PathEscape(v.ID),
	}
}

func ModrinthPlan(plan []modrinth.InstallFile) []InstallFile {
	files := make([]InstallFile, 0, len(plan))
	for _, item := range plan {
		files = append(files, InstallFile{Version: fromModrinth(item.Version), Kind: item.Kind, Name: item.File.Name, URL: item.File.URL, SHA1: item.File.Hashes["sha1"], Size: item.File.Size, Disabled: item.Disabled})
	}
	return files
}
