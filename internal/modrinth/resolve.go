package modrinth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"

	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type Installed struct {
	Version Version
	Enabled bool
}
type InstallFile struct {
	Version  Version
	Kind     resource.Kind
	File     File
	Disabled bool
}

func (c Client) Resolve(ctx context.Context, id string, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolve(ctx, []string{id}, filter, installed, false, false)
}

func (c Client) ResolveUpdates(ctx context.Context, ids []string, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolve(ctx, ids, filter, installed, true, false)
}

// ResolveReinstall resolves a selected root version even when that exact
// version is already installed. Its dependencies retain the normal update
// behavior and are reused when possible.
func (c Client) ResolveReinstall(ctx context.Context, id string, filter Filter, installed map[string]Installed) ([]InstallFile, error) {
	return c.resolve(ctx, []string{id}, filter, installed, true, true)
}

func (c Client) resolve(ctx context.Context, ids []string, filter Filter, installed map[string]Installed, replace, reinstallRoot bool) ([]InstallFile, error) {
	if len(ids) == 0 || len(ids) > 1000 || !filter.Kind.Valid() {
		return nil, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	var roots []Version
	var err error
	if len(ids) == 1 {
		var v Version
		v, err = c.Version(ctx, ids[0])
		roots = []Version{v}
	} else {
		roots, err = c.VersionsByID(ctx, ids)
	}
	if err != nil {
		return nil, err
	}
	wanted := make(map[string]Version, len(roots))
	for _, v := range roots {
		if previous, ok := wanted[v.ProjectID]; ok && previous.ID != v.ID {
			return nil, fmt.Errorf(locales.DependencyVersionConflict(), v.ProjectID)
		}
		wanted[v.ProjectID] = v
	}
	activeRoot := func(v Version) bool { old, ok := installed[v.ProjectID]; return !replace || !ok || old.Enabled }
	slices.SortStableFunc(roots, func(a, b Version) int {
		if activeRoot(a) == activeRoot(b) {
			return 0
		}
		if activeRoot(a) {
			return -1
		}
		return 1
	})
	chosen := make(map[string]Version)
	enabled := make(map[string]bool)
	kinds := make(map[string]resource.Kind)
	resourceKind := func(value string) resource.Kind {
		if filter.Kind == resource.DataPack && (value == "mod" || value == "datapack") {
			return resource.DataPack
		}
		return projectResourceKind(value)
	}
	if len(roots) > 1 {
		projects := make([]string, 0, len(wanted))
		for id := range wanted {
			projects = append(projects, id)
		}
		for start := 0; start < len(projects); start += 50 {
			batch := projects[start:min(start+50, len(projects))]
			raw, _ := json.Marshal(batch)
			body, err := c.Fetcher.GetLimited(ctx, baseURL+"/projects?"+url.Values{"ids": {string(raw)}}.Encode(), 8<<20)
			if err != nil {
				return nil, err
			}
			var metadata []struct {
				ID   string `json:"id"`
				Type string `json:"project_type"`
			}
			if err := json.Unmarshal(body, &metadata); err != nil {
				return nil, err
			}
			for _, project := range metadata {
				if _, exists := kinds[project.ID]; exists || !slices.Contains(batch, project.ID) {
					return nil, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
				}
				kinds[project.ID] = resourceKind(project.Type)
			}
			for _, id := range batch {
				if !kinds[id].Valid() {
					return nil, errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
				}
			}
		}
	}
	projectKind := func(id string) (resource.Kind, error) {
		if kind, ok := kinds[id]; ok {
			return kind, nil
		}
		var project struct {
			Type string `json:"project_type"`
		}
		if err := c.Fetcher.GetJSONLimited(ctx, baseURL+"/project/"+url.PathEscape(id), &project, 2<<20); err != nil {
			return "", err
		}
		kind := resourceKind(project.Type)
		if !kind.Valid() {
			return "", fmt.Errorf(locales.UnsupportedDependencyType(), project.Type)
		}
		kinds[id] = kind
		return kind, nil
	}
	var plan []InstallFile
	var visit func(Version, bool) error
	visit = func(v Version, active bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if previous, ok := chosen[v.ProjectID]; ok {
			if previous.ID != v.ID {
				return fmt.Errorf(locales.DependencyVersionConflict(), v.ProjectID)
			}
			if active && !enabled[v.ProjectID] {
				return fmt.Errorf(locales.DependencyIsDisabledEnableItFirst(), v.Name)
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
		kind, err := projectKind(v.ProjectID)
		if err != nil {
			return err
		}
		if _, root := wanted[v.ProjectID]; root && kind != filter.Kind {
			return errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
		if filter.Kind == resource.DataPack && kind != resource.DataPack {
			return errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
		candidate := filter
		candidate.Kind = kind
		if !v.Compatible(candidate) {
			return fmt.Errorf(locales.IsIncompatibleWithThisInstance(), v.Name)
		}
		if previous, ok := installed[v.ProjectID]; ok {
			if replace && active && !previous.Enabled {
				return fmt.Errorf(locales.DependencyIsDisabledEnableItFirst(), v.Name)
			}
			active = active || previous.Enabled
			if replace && previous.Version.ID != v.ID && !v.Published.After(previous.Version.Published) {
				return fmt.Errorf(locales.DependencyVersionConflict(), v.Name)
			}
		}
		chosen[v.ProjectID], enabled[v.ProjectID] = v, active
		for _, dep := range v.Dependencies {
			if dep.Type != "required" {
				continue
			}
			depKind := resource.Kind("")
			if dep.ProjectID != "" {
				depKind, err = projectKind(dep.ProjectID)
				if err != nil {
					return err
				}
			}
			depFilter := filter
			depFilter.Kind = depKind
			var required Version
			if forced, ok := wanted[dep.ProjectID]; ok {
				if dep.VersionID != "" && forced.ID != dep.VersionID {
					return fmt.Errorf(locales.DependencyVersionConflict(), dep.ProjectID)
				}
				required = forced
			} else if selected, ok := chosen[dep.ProjectID]; ok && (dep.VersionID == "" || selected.ID == dep.VersionID) {
				required = selected
			} else if old, ok := installed[dep.ProjectID]; ok && (old.Enabled || replace && !active) && (dep.VersionID == "" || old.Version.ID == dep.VersionID) && old.Version.Compatible(depFilter) {
				required = old.Version
			} else if dep.VersionID != "" {
				required, err = c.Version(ctx, dep.VersionID)
			} else if dep.ProjectID != "" {
				var versions []Version
				versions, err = c.Versions(ctx, dep.ProjectID, depFilter)
				if err == nil {
					if len(versions) == 0 {
						return fmt.Errorf(locales.NoCompatibleVersionForDependency(), dep.ProjectID)
					}
					required = versions[0]
					for _, version := range versions {
						if version.Channel == "release" {
							required = version
							break
						}
					}
				}
			} else {
				return fmt.Errorf(locales.HasDependenciesThatRequireManualDownload(), v.Name)
			}
			if err != nil {
				return err
			}
			if dep.ProjectID != "" && required.ProjectID != dep.ProjectID || dep.VersionID != "" && required.ID != dep.VersionID {
				return errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
			}
			if err := visit(required, active); err != nil {
				return err
			}
		}
		if old, ok := installed[v.ProjectID]; ok {
			force := reinstallRoot && wanted[v.ProjectID].ID == v.ID
			if old.Version.ID == v.ID && (old.Enabled || replace) && !force {
				return nil
			}
			if !replace {
				return fmt.Errorf(locales.IsInstalledEnableOrRemoveItFirst(), old.Version.Name)
			}
		}
		file, err := v.PrimaryFile(kind)
		if err != nil {
			return err
		}
		plan = append(plan, InstallFile{Version: v, Kind: kind, File: file, Disabled: !active})
		return nil
	}
	for _, v := range roots {
		if err := visit(v, activeRoot(v)); err != nil {
			return nil, err
		}
	}
	active := make(map[string]Version, len(installed)+len(chosen))
	for id, old := range installed {
		if old.Enabled {
			active[id] = old.Version
		}
	}
	for id, v := range chosen {
		if enabled[id] {
			active[id] = v
		} else {
			delete(active, id)
		}
	}
	for _, v := range active {
		for _, dep := range v.Dependencies {
			if dep.Type == "required" && dep.ProjectID == "" && dep.VersionID != "" {
				for id, old := range installed {
					if old.Version.ID != dep.VersionID {
						continue
					}
					if selected, touched := chosen[id]; touched && (!enabled[id] || selected.ID != dep.VersionID) {
						return nil, fmt.Errorf(locales.DependencyVersionConflict(), v.Name)
					}
				}
			}
			if dep.Type == "required" && dep.ProjectID != "" {
				if selected, touched := chosen[dep.ProjectID]; touched && (!enabled[dep.ProjectID] || dep.VersionID != "" && selected.ID != dep.VersionID) {
					return nil, fmt.Errorf(locales.DependencyVersionConflict(), v.Name)
				}
			}
			if dep.Type != "incompatible" {
				continue
			}
			for _, other := range active {
				if (dep.ProjectID == "" || other.ProjectID == dep.ProjectID) && (dep.VersionID == "" || other.ID == dep.VersionID) {
					return nil, fmt.Errorf(locales.ConflictsWithInstalled(), v.Name, other.Name)
				}
			}
		}
	}
	return plan, nil
}

func projectResourceKind(value string) resource.Kind {
	switch value {
	case "mod":
		return resource.Mod
	case "resourcepack":
		return resource.ResourcePack
	case "shader":
		return resource.ShaderPack
	case "world":
		return resource.World
	}
	return ""
}
