package resourcecatalog

import (
	"context"
	"errors"
	"sync"

	"github.com/UruhaLushia/tomori/internal/modrinth"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type UpdateCandidate struct {
	Origin         resource.Origin
	Version        Version
	Size, NextSize int64
}

type UpdateCandidates struct {
	Known      map[string]resource.Origin
	Updates    map[string]UpdateCandidate
	Duplicates map[string]bool
}

func (c Client) Updates(ctx context.Context, source Source, filter Filter, hashes []string, fingerprints []Fingerprint) (UpdateCandidates, error) {
	result := UpdateCandidates{Known: make(map[string]resource.Origin), Updates: make(map[string]UpdateCandidate), Duplicates: make(map[string]bool)}
	if !source.Valid() || !filter.Kind.Valid() || filter.GameVersion == "" || len(hashes) > 1000 || len(fingerprints) > 1000 {
		return result, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	if source == Modrinth {
		client := modrinth.Client{Fetcher: c.Fetcher}
		current, err := client.Identify(ctx, hashes)
		if err != nil {
			return result, err
		}
		latest, err := client.LatestReleases(ctx, hashes, filter)
		if err != nil {
			return result, err
		}
		for hash, old := range current {
			origin := resource.Origin{Source: string(Modrinth), ProjectID: old.ProjectID, VersionID: old.ID, Number: old.Number}
			if old.ID == "" || old.ProjectID == "" {
				continue
			}
			for _, file := range old.Files {
				if file.Hashes["sha1"] == hash && file.Size > 0 {
					result.Known[hash] = origin
					version, ok := latest[hash]
					if ok && version.ProjectID == old.ProjectID && version.ID != old.ID && version.Published.After(old.Published) {
						if next, err := version.PrimaryFile(filter.Kind); err == nil && next.Hashes["sha1"] != hash {
							result.Updates[hash] = UpdateCandidate{Origin: origin, Version: fromModrinth(version), Size: file.Size, NextSize: next.Size}
						}
					}
					break
				}
			}
		}
		return result, ctx.Err()
	}
	installed, err := c.IdentifyCurse(ctx, fingerprints)
	if err != nil {
		return result, err
	}
	type item struct {
		project string
		old     Installed
	}
	items := make([]item, 0, len(installed))
	for project, old := range installed {
		origin := resource.Origin{Source: string(CurseForge), ProjectID: project, VersionID: old.VersionID, Number: old.Number}
		result.Known[old.SHA1] = origin
		if old.Copies > 1 {
			result.Duplicates[project] = true
			continue
		}
		items = append(items, item{project, old})
	}
	ids := make([]string, 0, len(items))
	for _, job := range items {
		ids = append(ids, job.project)
	}
	projects, err := c.curseProjects(ctx, ids)
	if err != nil {
		return result, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var problem error
	next := 0
	var workers sync.WaitGroup
	for range min(4, len(items)) {
		workers.Go(func() {
			for {
				mu.Lock()
				if next == len(items) || ctx.Err() != nil {
					mu.Unlock()
					return
				}
				job := items[next]
				next++
				mu.Unlock()
				old := job.old.file
				project := projects[job.project]
				var err error
				var newest curseFile
				if classKind(project.ClassID) == filter.Kind {
					err = c.curseFiles(ctx, job.project, filter, func(file curseFile) {
						if file.Release == 1 && old != nil && file.Published.After(old.Published) && (newest.ID == 0 || file.Published.After(newest.Published)) {
							newest = file
						}
					})
				}
				mu.Lock()
				if err != nil {
					if problem == nil {
						problem = err
						cancel()
					}
				} else if newest.ID != 0 && old != nil && newest.ID != old.ID && newest.sha1() != old.sha1() {
					v := newest.version(project.website())
					if project.AllowDistribution != nil && !*project.AllowDistribution {
						v.Downloadable = false
					}
					result.Updates[job.old.SHA1] = UpdateCandidate{Origin: result.Known[job.old.SHA1], Version: v, Size: old.Size, NextSize: newest.Size}
				}
				mu.Unlock()
			}
		})
	}
	workers.Wait()
	if problem != nil {
		return result, problem
	}
	return result, ctx.Err()
}
