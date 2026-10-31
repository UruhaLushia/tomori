package app

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	downloadCategoryGame = "game"
	downloadCategoryPack = "pack"
)

func downloadResourceKind(category string) (resource.Kind, bool) {
	kind := resource.Kind(category)
	return kind, kind == resource.Mod || kind == resource.ResourcePack || kind == resource.ShaderPack || kind == resource.DataPack || kind == resource.World
}

func (a *app) downloadCategoryTabs(c *ui.Context) {
	ui.Row(c).Wrap().MinWidth(0).MaxWidthPercent(100).Gap(4).AlignItems(ui.Center).Children(func() {
		for _, item := range []struct {
			category string
			label    string
		}{
			{downloadCategoryGame, locales.GameVersion()},
			{downloadCategoryPack, locales.Modpack()},
			{string(resource.Mod), locales.Mods()},
			{string(resource.ResourcePack), locales.ResourcePacks()},
			{string(resource.ShaderPack), locales.Shaders()},
			{string(resource.World), locales.Worlds()},
			{string(resource.DataPack), locales.DataPacks()},
		} {
			c.Key("download-category-" + item.category)
			kit.NavTab(c, item.label, a.downloadCategory == item.category, func() { a.selectDownloadCategory(item.category) })
		}
	})
}

func (a *app) downloadHeader(c *ui.Context) {
	kit.PageHeader(c, locales.DownloadsPage(), "", func() { a.downloadCategoryTabs(c) }, func() { a.downloadHeaderActions(c) })
}

func (a *app) downloadHeaderActions(c *ui.Context) {
	if _, ok := downloadResourceKind(a.downloadCategory); ok && len(a.installedList) > 0 {
		if a.resources.data.EntriesLoading {
			kit.PresenceMotion(ui.Spinner(c).Size(16, 16)).Shrink(0)
		}
	} else if a.downloadCategory == downloadCategoryPack {
		kit.Button(c, locales.ImportALocalModpack(), a.openPackImport).Disabled(a.tasksFull() || a.pathBusy)
	}
}

func (a *app) selectDownloadCategory(category string) {
	if category == "" || category == a.downloadCategory {
		return
	}
	if a.packBrowser.data.FullPage && category != downloadCategoryPack {
		a.closeDownloadPackBrowser()
	}
	a.downloadCategory = category
	if category == downloadCategoryPack {
		a.openDownloadPackBrowser()
		return
	}
	kind, resourceCategory := downloadResourceKind(category)
	r := &a.resources
	if !resourceCategory {
		if r.data.FullPage && r.data.Open {
			a.closeResources()
		}
		return
	}
	if !r.data.FullPage || !r.data.Open {
		a.ensureDownloadResources()
		return
	}
	if r.data.Kind == kind && r.data.Online {
		return
	}
	if kind == resource.World {
		r.data.Source = resourcecatalog.CurseForge
	}
	r.request++
	r.data.ID, r.data.Kind, r.data.Online = r.request, kind, true
	r.data.Category = ""
	r.data.Project, r.data.Versions = nil, nil
	r.data.Search, r.data.Entries = resourcecatalog.SearchResult{}, nil
	r.data.Error, r.data.Status, r.data.Query = "", "", ""
	a.refreshResourceEntries()
	a.searchResources("", 0)
}

func (a *app) downloadVersions(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Gap(12).Children(func() {
		kit.Toolbar(c, func() {
			kit.Segmented(c, &a.downloadVersionFilter, locales.Release(), locales.Snapshot(), locales.Legacy()).OnChange(func() { a.downloadVersionsList = ui.ListState{} })
			ui.Spacer(c)
			kit.SearchField(c, &a.downloadVersionQuery, locales.SearchVersions()).OnChange(func() { a.downloadVersionsList = ui.ListState{} })
			kit.IconButton(c, kit.Refresh, false, a.loadManifest).Label(locales.RefreshVersions()).Tooltip(locales.RefreshVersions()).Disabled(a.manifestLoading)
		})
		versions := make([]minecraft.VersionSummary, 0, len(a.versions))
		latest := ""
		query := strings.ToLower(strings.TrimSpace(a.downloadVersionQuery))
		for _, version := range a.versions {
			channel := 2
			switch version.Type {
			case "release":
				channel = 0
			case "snapshot":
				channel = 1
			}
			if channel != a.downloadVersionFilter {
				continue
			}
			if latest == "" {
				latest = version.ID
			}
			if strings.Contains(strings.ToLower(version.ID), query) {
				versions = append(versions, version)
			}
		}
		kit.SectionHeader(c, []string{locales.Release(), locales.Snapshot(), locales.Legacy()}[a.downloadVersionFilter], func() { kit.Label(c, fmt.Sprintf(locales.Total(), len(versions)), kit.Caption) })
		switch {
		case a.manifestLoading && len(versions) == 0:
			kit.Loading(c)
		case a.manifestError != "" && len(versions) == 0:
			kit.EmptyState(c, kit.Download, locales.CouldNotLoadVersions(), a.manifestError, func() { kit.Button(c, locales.Retry(), a.loadManifest) })
		case len(versions) == 0:
			kit.EmptyState(c, kit.Download, locales.NoGameVersions(), locales.RefreshTheVersionListToInstall(), func() { kit.Button(c, locales.Reload(), a.loadManifest) })
		default:
			c.Key("download-version-list")
			ui.List(c, &a.downloadVersionsList, len(versions)).Grow(1).FillWidth().MinHeight(0).Padding(0, 0, kit.Pad(c), 0).
				ItemKey(func(i int) any {
					return versions[i].ID
				}).Scrollbars(false).Rows(func(row ui.ListRow) {
				c := row.Context
				version := versions[row.Index]
				progress := a.installStatus("game:" + version.ID)
				tags := version.ReleaseTime.Format("2006-01-02")
				if version.ReleaseTime.IsZero() {
					tags = ""
				}
				if progress.Active {
					tags = locales.Installing2() + " " + progress.Name + " · " + progress.Status
				}
				kit.ListRow(c, kit.Row{Title: version.ID, Tags: tags, Lead: func() {
					if progress.Active {
						ui.Box(c).Size(32, 32).Center().Children(func() { c.Key("install-spinner"); ui.Spinner(c).Size(16, 16) })
					} else {
						kit.IconBadge(c, kit.Cube, 32)
					}
				}, Badge: func() {
					if version.ID == latest {
						kit.Badge(c, locales.Latest(), kit.AccentTone)
					}
				}}, func(bool) {
					if progress.Active {
						kit.InstallTail(c, progress, 160)
						return
					}
					ui.Row(c).Width(kit.W(160)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
						count := 0
						for _, instance := range a.installedList {
							if instance.GameVersion == version.ID {
								count++
							}
						}
						if count > 0 {
							kit.Line(c, fmt.Sprintf(locales.InstancesInstalled(), count), kit.Caption)
						}
						kit.Button(c, locales.Install(), func() { a.openDownloadVersion(version.ID) }).Height(kit.TabH()).Disabled(a.tasksFull() || a.pathBusy)
					})
				})
			})
		}
	})
}

func (a *app) openDownloadVersion(version string) {
	if a.tasksFull() || a.pathBusy {
		return
	}
	a.openInstall(version)
}

func (a *app) downloadPacks(c *ui.Context) {
	a.packBrowser.data.InstallStatus = a.installStatus
	if a.packBrowser.data.Open && a.packBrowser.data.FullPage {
		a.pages.packBrowser.View(c, &a.packBrowser.data, a.downloadPackBrowserActions())
		return
	}
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		kit.EmptyState(c, kit.Cube, locales.OnlineModpacks(), locales.DownloadAModpackFromModrinthOrCurseForgeToCreateAnInstance(), func() {
			kit.Primary(c, locales.BrowseModpacks(), a.openPackBrowser).Disabled(a.tasksFull() || a.pathBusy)
		})
	})
}

func (a *app) installStatus(source string) kit.InstallProgress {
	for id, op := range a.operations {
		if op.sourceID == source && op.job.data.Active {
			fraction := op.job.data.Fraction()
			if op.job.data.Total == 0 {
				fraction = -1
			}
			return kit.InstallProgress{Active: true, Canceling: op.job.data.Canceling, Name: op.installName, Status: op.job.data.Status, Fraction: fraction, Cancel: func() { a.cancelOperation(id) }}
		}
	}
	return kit.InstallProgress{}
}

func (a *app) downloadNoTarget(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Children(func() {
		kit.EmptyState(c, kit.Cube, locales.NoAvailableInstances(), locales.CreateOrImportAnInstanceFirstThenDownloadResourcesForIt(), func() {
			kit.Primary(c, locales.NewInstance(), func() { a.openDownloadVersion("") })
		})
	})
}
