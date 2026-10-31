package app

import (
	"context"
	"os"
	"slices"
	"time"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) scanInstalled() {
	a.instancesReady = false
	a.icons.invalidate()
	if a.scanCancel != nil {
		a.scanCancel()
	}
	a.scanID++
	token := a.scanID
	directories := slices.Clone(a.cfg.Directories.Entries)
	budget := a.downloadBudget
	ctx, cancel := context.WithCancel(a.ctx)
	a.scanCancel = cancel
	a.workers.Go(func() {
		defer cancel()
		roots := make([]string, 0, len(directories))
		names := make(map[string]string, len(directories))
		for _, directory := range directories {
			roots = append(roots, directory.Path)
			names[directory.Path] = directory.Name
		}
		list, problems := minecraft.DiscoverDirectories(ctx, roots)
		for i := range list {
			list[i].SourceName = names[list[i].SourceDir]
			if list[i].PackSource.Provider == "" {
				release, err := budget.FilePermit(ctx)
				if err != nil {
					return
				}
				if root, err := os.OpenRoot(list[i].InstanceDir); err == nil {
					if header, err := modpack.ReadOriginHeader(ctx, root); err == nil {
						list[i].PackSource, list[i].PackFormat, list[i].PackVersion = header.Source, header.Pack.Format, header.Pack.Version
					}
					_ = root.Close()
				}
				release()
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			if token != a.scanID || a.ctx.Err() != nil || !slices.Equal(directories, a.cfg.Directories.Entries) {
				return
			}
			installed := make(map[string]bool, len(list))
			for _, v := range list {
				installed[v.Key()] = v.CanLaunch()
			}
			orderInstances(list, a.cfg.Directories.InstanceOrder)
			a.installed, a.installedList = installed, list
			a.instancesReady = true
			a.icons.retain(installed)
			a.normalizeCurrentVersion()
			for _, err := range problems {
				a.logf(locales.InstanceScan(), err)
			}
			a.drainLaunchRequests()
		})
	})
}

func (a *app) normalizeCurrentVersion() {
	if a.installed[a.cfg.Directories.SelectedInstance] {
		return
	}
	id := ""
	for _, instance := range a.installedList {
		if instance.CanLaunch() {
			id = instance.Key()
			break
		}
	}
	if a.cfg.Directories.SelectedInstance != id {
		a.cfg.Directories.SelectedInstance = id
		a.saveConfig()
	}
}

func (a *app) loadManifest() {
	if a.pathBusy && !a.components.data.Open {
		return
	}
	if a.manifestCancel != nil {
		a.manifestCancel()
	}
	a.manifestID++
	token := a.manifestID
	a.manifestLoading, a.manifestError = true, ""
	service := a.newService()
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	a.manifestCancel = cancel
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		manifest, err := service.Catalog(ctx)
		a.win.Update(func() {
			if token != a.manifestID || a.ctx.Err() != nil {
				return
			}
			a.manifestLoading = false
			if err != nil {
				a.manifestError = err.Error()
				a.logf(locales.VersionManifest2(), err)
			}
			if manifest != nil {
				a.versions = manifest.Versions
			}
		})
	})
}

func (a *app) summaryByID(id string) (minecraft.VersionSummary, bool) {
	for _, v := range a.versions {
		if v.ID == id {
			return v, true
		}
	}
	return minecraft.VersionSummary{}, false
}

func (a *app) currentVersion() string {
	if a.installed[a.cfg.Directories.SelectedInstance] {
		return a.cfg.Directories.SelectedInstance
	}
	for _, instance := range a.installedList {
		if instance.CanLaunch() {
			return instance.Key()
		}
	}
	return ""
}

func (a *app) instanceByKey(key string) (minecraft.Instance, bool) {
	for _, instance := range a.installedList {
		if instance.Key() == key {
			return instance, true
		}
	}
	return minecraft.Instance{}, false
}
