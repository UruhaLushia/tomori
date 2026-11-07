package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type resourceState struct {
	data           page.ResourcesData
	instance       minecraft.Instance
	request        uint64
	cancel         context.CancelFunc
	entriesID      uint64
	entriesCancel  context.CancelFunc
	missingID      uint64
	retryVersion   *resourcecatalog.Version
	retryPolicy    resource.InstallPolicy
	retryUpdates   []resourcecatalog.ResourceUpdate
	supplementTask uint64
}

func (a *app) openInstanceResources(key string, kind resource.Kind, embedded bool) {
	if a.pathBusy {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok {
		return
	}
	a.closeResources()
	r := &a.resources
	r.instance = instance
	r.data = page.ResourcesData{ID: r.request, Open: true, Embedded: embedded, Instance: instance.Name, Root: instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate), Kind: kind,
		GameVersion: instance.GameVersion, Loader: instance.Loader, CanInstallMods: instance.Loader != "" || len(instance.Loaders) > 0, Source: resourcecatalog.CurseForge}
	a.refreshResources()
}

func (a *app) openWorldPacks(file instancecontent.File) {
	key := a.instanceContent.data.Key
	instance, ok := a.instanceByKey(key)
	if !ok || a.pathBusy || a.instanceInUse(key) {
		return
	}
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	folder := filepath.Base(file.Path)
	if filepath.Clean(file.Path) != filepath.Join(root, "saves", folder) {
		return
	}
	found := false
	for _, candidate := range a.instanceContent.data.Summary.Worlds {
		if candidate.Path == file.Path {
			found = true
			break
		}
	}
	if !found {
		return
	}
	a.closeResources()
	r := &a.resources
	r.instance = instance
	r.data = page.ResourcesData{ID: r.request, Open: true, Instance: file.Name, Root: file.Path, World: folder, Kind: resource.DataPack, GameVersion: instance.GameVersion, Source: resourcecatalog.CurseForge}
	if world := a.instanceContent.data.Worlds[file.Path]; world.Version != "" {
		r.data.GameVersion = world.Version
	}
	a.refreshResources()
}

func (a *app) closeResources() {
	a.closeMissingResources()
	r := &a.resources
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	if r.entriesCancel != nil {
		r.entriesCancel()
		r.entriesCancel = nil
	}
	r.request++
	r.data = page.ResourcesData{}
	r.instance = minecraft.Instance{}
	a.pages.resources = page.Resources{}
	a.pages.instances.ReleaseResources()
}

func (a *app) resourceContext(timeout time.Duration) (context.Context, uint64) {
	r := &a.resources
	if r.cancel != nil {
		r.cancel()
	}
	r.request++
	ctx, cancel := context.WithTimeout(a.ctx, timeout)
	r.cancel = cancel
	r.data.Loading, r.data.Error = true, ""
	return ctx, r.request
}

func (a *app) changeResources(kind resource.Kind, online bool) {
	r := &a.resources
	if !r.data.Open || !kind.Valid() || r.data.World != "" && kind != resource.DataPack || (kind == r.data.Kind && online == r.data.Online) {
		return
	}
	previousKind := r.data.Kind
	r.data.Kind, r.data.Online = kind, online
	a.closeMissingResources()
	r.data.Project, r.data.Versions = nil, nil
	if kind != previousKind {
		r.data.Entries = nil
		r.data.Category = ""
	}
	r.data.Error, r.data.Status, r.data.Query = "", "", ""
	r.data.Search = resourcecatalog.SearchResult{}
	r.data.Missing, r.data.MissingOpen = nil, false
	if online {
		a.refreshResourceEntries()
		a.searchResources("", 0)
	} else {
		a.refreshResources()
	}
}

func (a *app) refreshResourceEntries() {
	r := &a.resources
	if !r.data.Open || r.data.World != "" || r.instance.Key() == "" {
		return
	}
	if r.entriesCancel != nil {
		r.entriesCancel()
	}
	r.entriesID++
	token := r.entriesID
	r.data.EntriesLoading = true
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	r.entriesCancel = cancel
	instance, root, kind := r.instance, r.data.Root, r.data.Kind
	directory, cache := a.store.Dir(), a.cfg.CacheDirectory()
	a.workers.Go(func() {
		defer cancel()
		list, err := resource.ScanWithOrigins(ctx, directory, root, kind)
		if err == nil && kind == resource.Mod {
			_ = resourcecatalog.LocalNames(ctx, cache, list)
		}
		a.win.Update(func() {
			r := &a.resources
			if token != r.entriesID || !r.data.Open || r.instance.Key() != instance.Key() || r.data.Root != root || r.data.Kind != kind || a.ctx.Err() != nil {
				return
			}
			r.entriesCancel = nil
			r.data.EntriesLoading = false
			if err == nil && list != nil {
				r.data.Entries = list
			}
		})
	})
}

func (a *app) changeResourceSource(query string) {
	r := &a.resources
	if !r.data.Open || !r.data.Online || !r.data.Source.Valid() {
		return
	}
	r.data.Category = ""
	a.closeMissingResources()
	r.data.Versions, r.data.Missing, r.data.Status, r.data.MissingOpen = nil, nil, "", false
	a.searchResources(query, 0)
}

func (a *app) changeResourceCategory(category string) {
	r := &a.resources
	if !r.data.Open || !r.data.Online || category == r.data.Category {
		return
	}
	r.data.Category = category
	r.data.Error, r.data.Status = "", ""
	r.data.Project, r.data.Versions = nil, nil
	a.searchResources(r.data.Query, 0)
}

func (a *app) refreshResources() {
	r := &a.resources
	if !r.data.Open {
		return
	}
	ctx, token := a.resourceContext(time.Minute)
	cancel := r.cancel
	root, kind := r.data.Root, r.data.Kind
	instance, world := r.instance, r.data.World
	directory := a.store.Dir()
	cache := a.cfg.CacheDirectory()
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var list []resource.Entry
		var err error
		if world != "" {
			list, err = service.ScanWorldPacks(ctx, instance, world)
		} else {
			list, err = resource.ScanWithOrigins(ctx, directory, root, kind)
		}
		if err == nil && kind == resource.Mod {
			_ = resourcecatalog.LocalNames(ctx, cache, list)
		}
		a.win.Update(func() {
			if token != a.resources.request || !a.resources.data.Open || a.ctx.Err() != nil {
				return
			}
			r := &a.resources
			r.data.Loading = false
			if err != nil {
				r.data.Error = err.Error()
				if list == nil {
					return
				}
			}
			r.data.Entries = list
			if a.instanceContent.data.Root == root {
				count := instancecontent.Count{Total: len(list)}
				for _, entry := range list {
					if entry.Enabled {
						count.Enabled++
					}
				}
				if a.instanceContent.data.Summary.Resources == nil {
					a.instanceContent.data.Summary.Resources = make(map[resource.Kind]instancecontent.Count)
				}
				a.instanceContent.data.Summary.Resources[kind] = count
			}
		})
	})
}

func (a *app) openResourceOrigin(entry resource.Entry, origin resource.Origin) {
	r := &a.resources
	if !r.data.Open || !resourcecatalog.Source(origin.Source).Valid() || !slices.Contains(entry.Origins, origin) {
		return
	}
	r.data.Online, r.data.Source = true, resourcecatalog.Source(origin.Source)
	r.data.Error, r.data.Status = "", ""
	r.data.Query, r.data.Search = "", resourcecatalog.SearchResult{}
	a.resourceVersions(resourcecatalog.Project{Source: r.data.Source, ID: origin.ProjectID, Title: entry.Name})
}

func (a *app) searchResources(query string, offset int) {
	r := &a.resources
	if !r.data.Open {
		return
	}
	a.closeMissingResources()
	r.data.Query = query
	r.data.Project = nil
	r.data.Versions, r.data.Missing, r.data.MissingOpen = nil, nil, false
	r.data.SearchOffset = offset
	ctx, token := a.resourceContext(time.Minute)
	cancel := r.cancel
	instance, kind := r.instance, r.data.Kind
	instance.GameVersion = r.data.GameVersion
	source := r.data.Source
	category := r.data.Category
	compatibility := r.data.Compatibility
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		result, err := service.SearchResources(ctx, instance, kind, source, compatibility, category, query, offset)
		a.win.Update(func() {
			if token != a.resources.request || !a.resources.data.Open || a.ctx.Err() != nil {
				return
			}
			r := &a.resources
			r.data.Loading = false
			if err != nil {
				r.data.Error = err.Error()
				return
			}
			if offset > 0 {
				previous := len(r.data.Search.Hits)
				seen := make(map[string]bool, len(r.data.Search.Hits))
				for _, project := range r.data.Search.Hits {
					seen[project.ID] = true
				}
				for _, project := range result.Hits {
					if !seen[project.ID] {
						r.data.Search.Hits = append(r.data.Search.Hits, project)
						seen[project.ID] = true
					}
				}
				r.data.Search.Total = result.Total
				r.data.Search.Offset = offset
				if len(r.data.Search.Hits) == previous || offset+len(result.Hits) >= result.Total {
					r.data.Search.Total = len(r.data.Search.Hits)
				}
			} else {
				r.data.Search = result
				r.data.SearchRevision++
				r.data.Icons = make(map[string]*ui.Bitmap)
			}
		})
		if err != nil {
			return
		}
		translated := service.TranslateDescriptions(ctx, result.Hits)
		if len(translated) > 0 {
			a.win.Update(func() {
				if token != a.resources.request || !a.resources.data.Open || a.ctx.Err() != nil {
					return
				}
				for _, project := range translated {
					for i := range a.resources.data.Search.Hits {
						if a.resources.data.Search.Hits[i].ID == project.ID {
							a.resources.data.Search.Hits[i].TranslatedDescription = project.TranslatedDescription
						}
					}
				}
			})
		}
		jobs := make(chan resourcecatalog.Project, len(result.Hits))
		for _, project := range result.Hits {
			if project.IconURL != "" {
				jobs <- project
			}
		}
		close(jobs)
		var group sync.WaitGroup
		for range 4 {
			group.Go(func() {
				for project := range jobs {
					if ctx.Err() != nil {
						return
					}
					cover, err := service.PackCover(ctx, project.IconURL)
					if err != nil {
						continue
					}
					bitmap := ui.NewBitmap(cover)
					a.win.Update(func() {
						if token == a.resources.request && a.resources.data.Open && a.ctx.Err() == nil {
							if a.resources.data.Icons == nil {
								a.resources.data.Icons = make(map[string]*ui.Bitmap)
							}
							a.resources.data.Icons[project.ID] = bitmap
						}
					})
				}
			})
		}
		group.Wait()
	})
}

func (a *app) resourceVersions(project resourcecatalog.Project) {
	r := &a.resources
	if !r.data.Open || project.Source != r.data.Source {
		return
	}
	r.data.Project, r.data.Versions = &project, nil
	ctx, token := a.resourceContext(time.Minute)
	cancel := r.cancel
	kind := r.data.Kind
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		versions, err := service.ResourceVersions(ctx, kind, project)
		a.win.Update(func() {
			if token != a.resources.request || !a.resources.data.Open || a.ctx.Err() != nil {
				return
			}
			r := &a.resources
			r.data.Loading = false
			if err != nil {
				r.data.Error = err.Error()
				return
			}
			r.data.Versions = versions
		})
	})
}

func (a *app) backResourceSearch() {
	a.closeMissingResources()
	r := &a.resources
	if r.cancel != nil {
		r.cancel()
	}
	r.request++
	r.data.Project, r.data.Versions, r.data.Error, r.data.Loading = nil, nil, "", false
	r.data.Missing, r.data.MissingOpen = nil, false
}

func (a *app) resourceEditable() bool {
	r := &a.resources
	if !r.data.Open || a.tasksFull() || a.pathBusy || a.instanceInUse(r.instance.Key()) {
		return false
	}
	if r.data.FullPage && r.instance.Key() == "" {
		return true
	}
	instance, ok := a.instanceByKey(r.instance.Key())
	root := instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate)
	if r.data.World != "" {
		root = filepath.Join(root, "saves", r.data.World)
	}
	if !ok || root != r.data.Root {
		r.data.Error = locales.InstanceDirectoryChangedReopenResourceManager()
		return false
	}
	return true
}

func (a *app) mutateResources(work func(context.Context, string, resource.Kind, *launcher.Service) (string, error)) {
	if !a.resourceEditable() {
		return
	}
	r := &a.resources
	if r.cancel != nil {
		r.cancel()
	}
	r.request++
	r.data.Loading = false
	r.data.Error, r.data.Status = "", ""
	root, kind := r.data.Root, r.data.Kind
	viewID := r.data.ID
	ctx, token := a.beginTask(locales.ResourceManager()+" · "+r.instance.Name, r.instance.Key(), root)
	if token == 0 {
		return
	}
	service := a.newService()
	a.workers.Go(func() {
		defer service.Close()
		status, err := work(ctx, root, kind, service)
		a.win.Update(func() {
			r := &a.resources
			if a.ctx.Err() != nil || r.data.ID != viewID || r.data.Root != root || r.data.Kind != kind {
				return
			}
			r.data.Status = status
			if err != nil {
				r.data.Error = err.Error()
				a.logf(locales.ResourceManager2(), err)
			}
			// Refresh even after a partial batch, preserving its error message.
			message := r.data.Error
			if r.data.Open && !r.data.Online {
				a.refreshResources()
				r.data.Error = message
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) toggleResources(entries []resource.Entry, enabled bool) {
	entries = slices.Clone(entries)
	instance, world := a.resources.instance, a.resources.data.World
	a.mutateResources(func(ctx context.Context, root string, kind resource.Kind, service *launcher.Service) (string, error) {
		if world != "" {
			action := "disable"
			if enabled {
				action = "enable"
			}
			_, err := service.ChangeWorldPacks(ctx, instance, world, action, entries)
			return "", err
		}
		var problems []error
		count := 0
		for _, entry := range entries {
			if entry.Enabled == enabled {
				continue
			}
			if err := resource.Toggle(ctx, root, kind, entry); err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", entry.File, err))
			} else {
				count++
			}
		}
		label := locales.Disable()
		if enabled {
			label = locales.Enable()
		}
		return fmt.Sprintf(locales.Message2(), label, count, kind.Label()), errors.Join(problems...)
	})
}

func (a *app) removeResources(entries []resource.Entry) {
	entries = slices.Clone(entries)
	instance, world := a.resources.instance, a.resources.data.World
	a.mutateResources(func(ctx context.Context, root string, kind resource.Kind, service *launcher.Service) (string, error) {
		if world != "" {
			_, err := service.ChangeWorldPacks(ctx, instance, world, "delete", entries)
			return "", err
		}
		var problems []error
		count := 0
		for _, entry := range entries {
			if err := resource.Remove(ctx, root, kind, entry); err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", entry.File, err))
			} else {
				count++
			}
		}
		return fmt.Sprintf(locales.Deleted(), count, kind.Label()), errors.Join(problems...)
	})
}

func (a *app) importResourcePaths(paths []string) {
	paths = slices.Clone(paths)
	instance, world := a.resources.instance, a.resources.data.World
	a.mutateResources(func(ctx context.Context, root string, kind resource.Kind, service *launcher.Service) (string, error) {
		var count int
		var err error
		if world != "" {
			count, err = service.ImportWorldPacks(ctx, instance, world, paths)
		} else {
			count, err = resource.Import(ctx, root, kind, paths)
		}
		return fmt.Sprintf(locales.Imported2(), count, kind.Label()), err
	})
}

func (a *app) importResources() {
	if !a.resourceEditable() {
		return
	}
	path, _ := mygo.App.Path(mygo.PathDownloads)
	if path == "" {
		path = a.resources.data.Root
	}
	a.openDirectoryPicker(pickerResource, path)
}

func (a *app) installResource(version resourcecatalog.Version, policy resource.InstallPolicy, targetKey string) {
	if !a.resourceEditable() {
		return
	}
	r := &a.resources
	if r.data.Project == nil || version.Source != r.data.Source || version.ProjectID != r.data.Project.ID {
		return
	}
	instance := r.instance
	if targetKey != "" {
		var ok bool
		instance, ok = a.instanceByKey(targetKey)
		if !ok {
			return
		}
	}
	if instance.Key() == "" {
		return
	}
	if r.data.Kind == resource.Mod && len(instance.Loaders) == 0 && instance.Loader == "" {
		r.data.Error = locales.NoModLoaderInstalled()
		return
	}
	kind, viewID := r.data.Kind, r.data.ID
	world := r.data.World
	root := instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate)
	if world != "" {
		root = r.data.Root
		instance.GameVersion = r.data.GameVersion
	}
	if a.instanceInUse(instance.Key()) {
		r.data.Error = instance.Name + " · " + locales.InstanceIsInUse()
		return
	}
	ctx, service, token := a.beginOperation(locales.Install2()+version.Name+" · "+instance.Name, instance.Key(), root)
	if token == 0 {
		return
	}
	progress := a.progressHandler(token)
	a.closeMissingResources()
	r.data.Error, r.data.Status = "", ""
	a.workers.Go(func() {
		defer service.Close()
		var count int
		var err error
		if world != "" {
			count, err = service.InstallWorldPack(ctx, instance, world, version, progress)
		} else {
			count, err = service.InstallResourceWithPolicy(ctx, instance, kind, version, policy, progress)
		}
		a.win.Update(func() {
			if !a.operationCurrent(token) || a.ctx.Err() != nil {
				return
			}
			current := a.resources.data.ID == viewID && a.resources.data.Kind == kind && (a.resources.data.Root == root || a.resources.data.FullPage)
			if err != nil {
				if current && a.resources.data.Source == version.Source && !errors.Is(err, context.Canceled) {
					a.resources.data.Error = err.Error()
					if missing, ok := errors.AsType[*resourcecatalog.MissingFilesError](err); ok {
						a.showMissingResources(missing.Files, &version, nil, policy)
					}
				}
			} else {
				a.logf(locales.InstalledFiles(), version.Name, count)
				if current {
					a.resources.data.Status = version.Name + locales.Installed() + " · " + instance.Name
					if a.resources.data.Open && !a.resources.data.Online {
						a.refreshResources()
					}
				}
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) resourcesView(c *ui.Context) {
	r := &a.resources
	if r.data.Embedded || r.data.FullPage {
		return
	}
	r.data.Busy = a.tasksFull() || a.pathBusy || a.instanceInUse(r.instance.Key())
	a.pages.resources.View(c, a.resourceViewData(), a.resourceActions())
}

func (a *app) resourceActions() page.ResourcesActions {
	r := &a.resources
	return page.ResourcesActions{
		AssessUpdates: a.assessResourceSpace,
		Close:         a.closeResources, Refresh: a.refreshResources, Import: a.importResources,
		ImportDirectory: func() {
			if a.resourceEditable() && r.data.World != "" {
				a.openDirectoryPicker(pickerResourceDirectory, r.data.Root)
			}
		},
		OpenDirectory: func() {
			if path, err := resource.Directory(r.data.Root, r.data.Kind); err == nil {
				a.openDirectory(path)
			}
		},
		Change: a.changeResources, Search: a.searchResources, Versions: a.resourceVersions, Back: a.backResourceSearch,
		SourceChanged: a.changeResourceSource, CategoryChanged: a.changeResourceCategory,
		Origin:  a.openResourceOrigin,
		Install: a.installResource, Toggle: a.toggleResources, Remove: a.removeResources,
		OpenUpdates: a.openResourceUpdates, CloseUpdates: a.closeResourceUpdates, CheckUpdates: a.checkResourceUpdates, BackUpdates: a.backResourceUpdates,
		Update: a.updateResources, UpdateDetails: a.resourceUpdateDetails,
		Supplement: a.browseResourceSupplement, RetryMissing: a.retryMissingResources, CloseMissing: a.closeMissingResources,
	}
}
