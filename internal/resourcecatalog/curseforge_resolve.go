package resourcecatalog

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

func (f curseFile) sha1() string {
	for _, hash := range f.Hashes {
		if hash.Algo == 1 {
			return strings.ToLower(hash.Value)
		}
	}
	return ""
}

func (c Client) IdentifyCurse(ctx context.Context, hashes []Fingerprint) (map[string]Installed, error) {
	installed := make(map[string]Installed)
	counts := make(map[string]int, len(hashes))
	active := make(map[string]bool, len(hashes))
	sizes := make(map[string]int64, len(hashes))
	for _, hash := range hashes {
		counts[hash.SHA1]++
		active[hash.SHA1] = active[hash.SHA1] || hash.Enabled
		sizes[hash.SHA1] = hash.Size
	}
	seen := make(map[string]bool, len(hashes))
	for start := 0; start < len(hashes); start += 200 {
		batch := hashes[start:min(start+200, len(hashes))]
		values := make([]uint32, 0, len(batch))
		byHash := make(map[string]bool, len(batch))
		for _, f := range batch {
			values = append(values, f.Value)
			byHash[f.SHA1] = active[f.SHA1]
		}
		var response struct {
			Data struct {
				Exact []struct{ File curseFile } `json:"exactMatches"`
			}
		}
		if err := c.Fetcher.PostJSONLimited(ctx, curseAPI+"/fingerprints/432", map[string]any{"fingerprints": values}, &response, 8<<20); err != nil {
			return nil, err
		}
		for _, match := range response.Data.Exact {
			f := match.File
			if f.ID <= 0 || f.ProjectID <= 0 || f.sha1() == "" || sizes[f.sha1()] > 0 && sizes[f.sha1()] != f.Size {
				continue
			}
			if enabled, known := byHash[f.sha1()]; known {
				if seen[f.sha1()] {
					continue
				}
				seen[f.sha1()] = true
				project := strconv.Itoa(f.ProjectID)
				previous, exists := installed[project]
				if exists && previous.Enabled && enabled && previous.VersionID != strconv.Itoa(f.ID) {
					return nil, fmt.Errorf(locales.MultipleVersionsOfAreEnabledRemoveDuplicatesFirst(), f.Name)
				}
				if !exists || !previous.Enabled {
					installed[project] = Installed{VersionID: strconv.Itoa(f.ID), Enabled: enabled, SHA1: f.sha1(), Number: f.Filename, file: &f, Copies: previous.Copies}
				}
				current := installed[project]
				current.Copies += counts[f.sha1()]
				installed[project] = current
			}
		}
	}
	return installed, nil
}

func (c Client) curseFile(ctx context.Context, project, id string) (curseFile, error) {
	if !validID(project) || !validID(id) {
		return curseFile{}, errors.New(locales.InvalidCurseforgeFileIdAlt())
	}
	var response struct{ Data curseFile }
	if err := c.Fetcher.GetJSONLimited(ctx, curseAPI+"/mods/"+project+"/files/"+id, &response, 8<<20); err != nil {
		return curseFile{}, err
	}
	if strconv.Itoa(response.Data.ID) != id || strconv.Itoa(response.Data.ProjectID) != project {
		return curseFile{}, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
	}
	return response.Data, nil
}

func (c Client) ResolveCurse(ctx context.Context, version Version, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolveCurse(ctx, []Version{version}, filter, installed, false, false)
}

func (c Client) ResolveCurseUpdates(ctx context.Context, versions []Version, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolveCurse(ctx, versions, filter, installed, true, false)
}

// ResolveCurseReinstall resolves a selected root version even when that exact
// version is already installed. Dependencies still use the normal update
// behavior.
func (c Client) ResolveCurseReinstall(ctx context.Context, version Version, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolveCurse(ctx, []Version{version}, filter, installed, true, true)
}

func (c Client) resolveCurse(ctx context.Context, versions []Version, filter Filter, installed map[string]Installed, replace, reinstallRoot bool) ([]InstallFile, error) {
	if len(versions) == 0 || len(versions) > 1000 || !filter.Kind.Valid() {
		return nil, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	wanted := make(map[string]curseFile, len(versions))
	roots := make([]curseFile, 0, len(versions))
	files, err := c.curseFilesByID(ctx, versions)
	if err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version.Source != CurseForge {
			return nil, errors.New(locales.InvalidResourceSourceOrType())
		}
		file := files[version.ID]
		if old, ok := wanted[version.ProjectID]; ok && old.ID != file.ID {
			return nil, fmt.Errorf(locales.DependencyVersionConflict(), version.ProjectID)
		}
		wanted[version.ProjectID] = file
		roots = append(roots, file)
	}
	activeRoot := func(f curseFile) bool {
		old, ok := installed[strconv.Itoa(f.ProjectID)]
		return !replace || !ok || old.Enabled
	}
	slices.SortStableFunc(roots, func(a, b curseFile) int {
		if activeRoot(a) == activeRoot(b) {
			return 0
		}
		if activeRoot(a) {
			return -1
		}
		return 1
	})
	chosen := make(map[string]curseFile)
	enabled := make(map[string]bool)
	projectIDs := make([]string, 0, len(wanted))
	for id := range wanted {
		projectIDs = append(projectIDs, id)
	}
	projects, err := c.curseProjects(ctx, projectIDs)
	if err != nil {
		return nil, err
	}
	getProject := func(id string) (curseProject, error) {
		if p, ok := projects[id]; ok {
			return p, nil
		}
		p, err := c.curseProject(ctx, id)
		if err == nil {
			projects[id] = p
		}
		return p, err
	}
	var plan []InstallFile
	var visit func(curseFile, bool) error
	visit = func(f curseFile, active bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := strconv.Itoa(f.ProjectID)
		if previous, ok := chosen[id]; ok {
			if previous.ID != f.ID {
				return fmt.Errorf(locales.DependencyVersionConflict(), id)
			}
			if active && !enabled[id] {
				return fmt.Errorf(locales.DependencyIsDisabledEnableItFirst(), f.Name)
			}
			return nil
		}
		limit := 100
		if replace {
			limit = min(1000, max(100, len(roots)+100))
		}
		if len(chosen) >= limit {
			return errors.New(locales.DependencyCountExceedsLimit())
		}
		project, err := getProject(id)
		if err != nil {
			return err
		}
		kind := classKind(project.ClassID)
		if filter.Kind == resource.DataPack && kind != resource.DataPack || filter.Kind != resource.DataPack && kind == resource.DataPack {
			return errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
		if _, root := wanted[id]; root && kind != filter.Kind {
			return errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
		current := filter
		current.Kind = kind
		if !f.compatible(current) {
			return fmt.Errorf(locales.IsIncompatibleWithThisInstance(), f.Name)
		}
		if previous, ok := installed[id]; ok {
			if replace && active && !previous.Enabled {
				return fmt.Errorf(locales.DependencyIsDisabledEnableItFirst(), project.Name)
			}
			active = active || previous.Enabled
		}
		chosen[id], enabled[id] = f, active
		for _, dependency := range f.Dependencies {
			if dependency.Relation != 3 {
				continue
			}
			depID := strconv.Itoa(dependency.ProjectID)
			var required curseFile
			if forced, ok := wanted[depID]; ok {
				required = forced
			} else if selected, ok := chosen[depID]; ok {
				required = selected
			} else {
				depProject, projectErr := getProject(depID)
				if projectErr != nil {
					return projectErr
				}
				depFilter := filter
				depFilter.Kind = classKind(depProject.ClassID)
				if old, ok := installed[depID]; ok && (old.Enabled || replace && !active) {
					if old.file != nil {
						required = *old.file
					} else {
						required, err = c.curseFile(ctx, depID, old.VersionID)
						if err != nil {
							return err
						}
					}
					if !required.compatible(depFilter) {
						required = curseFile{}
					}
				}
				if required.ID == 0 {
					err = c.curseFiles(ctx, depID, depFilter, func(file curseFile) {
						release, selectedRelease := file.Release == 1, required.Release == 1
						if required.ID == 0 || release && !selectedRelease || release == selectedRelease && file.Published.After(required.Published) {
							required = file
						}
					})
					if err != nil {
						return err
					}
					if required.ID == 0 {
						return fmt.Errorf(locales.NoCompatibleVersionForDependency(), depProject.Name)
					}
				}
			}
			if err := visit(required, active); err != nil {
				return err
			}
		}
		if existing, ok := installed[id]; ok {
			force := reinstallRoot && wanted[id].ID == f.ID
			if existing.VersionID == strconv.Itoa(f.ID) && (existing.Enabled || replace) && !force {
				return nil
			}
			if !replace {
				return fmt.Errorf(locales.IsInstalledEnableOrRemoveItFirst(), project.Name)
			}
		}
		version := f.version(project.website())
		manual := f.URL == "" || project.AllowDistribution != nil && !*project.AllowDistribution
		if manual {
			f.URL = ""
			version.Downloadable = false
		}
		plan = append(plan, InstallFile{Version: version, Kind: kind, Name: f.Filename, URL: f.URL, SHA1: f.sha1(), Size: f.Size, Disabled: !active, Manual: manual})
		return nil
	}
	for _, root := range roots {
		if err := visit(root, activeRoot(root)); err != nil {
			return nil, err
		}
	}
	if replace {
		for id, old := range installed {
			if _, changed := chosen[id]; changed || !old.Enabled {
				continue
			}
			file := old.file
			if file == nil {
				metadata, err := c.curseFile(ctx, id, old.VersionID)
				if err != nil {
					return nil, err
				}
				file = &metadata
			}
			for _, dependency := range file.Dependencies {
				depID := strconv.Itoa(dependency.ProjectID)
				if _, changed := chosen[depID]; !changed {
					continue
				}
				if dependency.Relation == 5 && enabled[depID] || dependency.Relation == 3 && !enabled[depID] {
					return nil, fmt.Errorf(locales.HasConflictingDependencies(), file.Name)
				}
			}
		}
	}
	for id, file := range chosen {
		if !enabled[id] {
			continue
		}
		for _, dependency := range file.Dependencies {
			if dependency.Relation != 5 {
				continue
			}
			depID := strconv.Itoa(dependency.ProjectID)
			if _, ok := chosen[depID]; ok {
				if enabled[depID] {
					return nil, fmt.Errorf(locales.HasConflictingDependencies(), file.Name)
				}
			} else if old, ok := installed[depID]; ok && old.Enabled {
				return nil, fmt.Errorf(locales.ConflictsWithInstalled(), file.Name, depID)
			}
		}
	}
	return plan, nil
}

func (c Client) curseFilesByID(ctx context.Context, versions []Version) (map[string]curseFile, error) {
	result := make(map[string]curseFile, len(versions))
	if len(versions) == 1 {
		v := versions[0]
		if v.Source != CurseForge {
			return nil, errors.New(locales.InvalidResourceSourceOrType())
		}
		f, err := c.curseFile(ctx, v.ProjectID, v.ID)
		if err != nil {
			return nil, err
		}
		result[v.ID] = f
		return result, nil
	}
	for start := 0; start < len(versions); start += 50 {
		batch := versions[start:min(start+50, len(versions))]
		ids := make([]int, 0, len(batch))
		wanted := make(map[int]string, len(batch))
		for _, v := range batch {
			if v.Source != CurseForge || !validID(v.ID) || !validID(v.ProjectID) {
				return nil, errors.New(locales.InvalidCurseforgeFileIdAlt())
			}
			id, _ := strconv.Atoi(v.ID)
			if project, exists := wanted[id]; exists && project != v.ProjectID {
				return nil, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
			}
			if _, exists := wanted[id]; !exists {
				ids = append(ids, id)
			}
			wanted[id] = v.ProjectID
		}
		var response struct{ Data []curseFile }
		if err := c.Fetcher.PostJSONLimited(ctx, curseAPI+"/mods/files", map[string]any{"fileIds": ids}, &response, 8<<20); err != nil {
			return nil, err
		}
		seen := make(map[int]bool, len(response.Data))
		for _, file := range response.Data {
			if wanted[file.ID] != strconv.Itoa(file.ProjectID) || seen[file.ID] {
				return nil, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
			}
			seen[file.ID] = true
			result[strconv.Itoa(file.ID)] = file
		}
		if len(seen) != len(wanted) {
			return nil, errors.New(locales.CurseForgeFileDoesNotMatchManifest())
		}
	}
	return result, nil
}
