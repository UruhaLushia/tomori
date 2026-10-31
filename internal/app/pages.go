package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"time"
	"github.com/UruhaLushia/tomori/internal/minecraft"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launchrequest"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type pageState struct {
	home             page.HomeState
	instances        page.Instances
	components       page.Components
	instanceSettings page.InstanceSettings
	game             page.Game
	download         page.Download
	logs             page.Logs
	appearance       page.Appearance
	downloadTasks    page.DownloadTasks
	directoryPicker  page.DirectoryPicker
	packBrowser      page.PackBrowser
	packUpdate       page.PackUpdate
	resources        page.Resources
}

func (a *app) homePage(c *ui.Context) {
	services := c.Services()
	a.refreshHome()
	identity, _ := a.cfg.SelectedAccount()
	instance, _ := a.instanceByKey(a.currentVersion())
	c.After(time.Second)
	recent := a.homeRecent()
	for _, r := range recent {
		a.loadInstanceIcon(r.Instance)
	}
	activity := a.instanceActivity(instance.Key())
	progress := float64(-1)
	since, otherSince := time.Time{}, time.Time{}
	otherRunning, busyStatus := "", ""
	if session := a.games[instance.Key()]; session != nil {
		since = session.playedAt
		if op := a.operations[session.token]; op != nil && (op.job.data.Total > 0 || len(op.job.data.Files) > 0) {
			progress = op.job.data.Fraction()
		}
	}
	for key, session := range a.games {
		if key != instance.Key() && session.activity.Running {
			otherRunning, otherSince = session.name, session.playedAt
			break
		}
	}
	for _, op := range a.operations {
		if op.job.data.Active {
			busyStatus = op.job.data.Status
			break
		}
	}
	if a.authBusy {
		busyStatus = locales.SigningIn() + " " + identity.Name + "…"
	}
	failure := ""
	if a.home.failureKey == instance.Key() {
		failure = a.home.failure
	}
	accounts := make([]kit.AccountItem, 0, len(a.cfg.Accounts.Entries))
	for _, entry := range a.cfg.Accounts.Entries {
		kind := entry.Label()
		if entry.Kind == account.Yggdrasil {
			kind = kit.MetaLine(entry.ServerName, entry.ServerURL)
		}
		accounts = append(accounts, kit.AccountItem{ID: entry.ID, Name: entry.Name, Kind: kind, Avatar: a.avatars[entry.ID].bitmap, Initial: entry.Initial(), Current: entry.ID == identity.ID})
	}
	a.pages.home.View(c, page.HomeData{
		Current:    instance,
		Instances:  a.installedList,
		Icons:      a.icons.bitmaps,
		Recent:     recent,
		Statistics: a.homeStatistics(instance.Key()),
		Loading:    a.home.cancel != nil,
		Account:    identity,
		Avatar:     a.avatars[identity.ID].bitmap,
		Accounts:   accounts,
		Busy:       a.tasksFull() || a.pathBusy || a.authBusy || a.instanceInUse(instance.Key()),
		Launching:  activity.Active,
		Running:    activity.Running,
		Finishing:  activity.Finishing,
		Canceling:  activity.Canceling,
		Status:     activity.Status,
		Progress:   progress, Since: since, OtherRunning: otherRunning, OtherSince: otherSince,
		BusyStatus: busyStatus, Failure: failure, FailureAction: a.home.failureAction, Activity: a.instanceActivity,
		Usage: page.HomeUsage{CPUTotal: a.home.cpu, CPUGame: a.home.gameCPU, CPULauncher: a.home.launcherCPU, MemUsedMiB: a.memory.UsedMiB, MemGameMiB: a.home.gameMiB, MemTotalMiB: a.memory.TotalMiB, LauncherMiB: a.home.launcherMiB, Ready: a.home.cpuReady},
	}, page.HomeActions{
		OpenLibrary:   func() { a.goPage(pageInstances) }, // nav:link
		OpenDirectory: func() { a.openDirectory(instance.InstanceDir) },
		Select:        a.selectVersion,
		OpenDetails: func() {
			a.goPage(pageInstances) // nav:link
			a.pages.instances.Open(instance.Key())
		},
		OpenRecent:     a.openHomeRecent,
		LaunchRecent:   a.launchHomeRecent,
		LoadIcon:       a.loadInstanceIcon,
		SelectAccount:  a.selectAccount,
		AddAccount:     a.dialogs.accounts.ShowAdd,
		ManageAccounts: func() { a.goPage(pageAccount) }, // nav:link
		Launch:         a.launch,
		Stop:           func() { a.stopInstance(instance.Key()) },
		New:            func() { a.openInstall("") },
		LaunchInstance: a.launchVersion,
		Menu: func(c *ui.Context, trigger ui.Element) {
			services := c.Services()
			page.InstanceContextMenu(c, trigger, instance, page.InstancesData{Busy: a.tasksFull() || a.pathBusy, InUse: a.instanceInUse, Activity: a.instanceActivity}, page.InstancesActions{
				Settings: a.openInstanceSettings, RefreshContent: a.refreshHome,
				Open: a.openDirectory, Repair: a.repair, Edit: a.openInstanceEdit, Copy: a.openInstanceCopy, Shortcut: a.openInstanceShortcut,
				CopyLink: func(key string) { services.WriteClipboard(launchrequest.Link(key)) }, Export: a.openPackExport, UpdatePack: a.openPackUpdate, AssociatePack: a.openPackAssociate, RestorePack: a.openPackRestore, Components: a.openComponents,
			}, func() { a.goPage(pageInstances); a.pages.instances.Open(instance.Key()) }) // nav:link
		},
		Recover: func() {
			if a.home.failureAction == "login" {
				a.dialogs.accounts.Reauthenticate(identity, a.accountActions(services))
			} else {
				a.repair(instance.Key())
			}
		},
		ViewLog: func() {
			a.goPage(pageLogs) // nav:link
			if a.home.failureLog != "" {
				a.chooseLogHistory(a.home.failureLog)
			}
		},
	})
}

func (a *app) instancesPage(c *ui.Context) {
	services := c.Services()
	if a.cfg.Appearance.LibraryView == 1 {
		a.refreshHome()
	}
	a.pages.instances.SetViewMode(a.cfg.Appearance.LibraryView, len(a.installedList) > 0)
	wasDetail := a.pages.instances.DetailOpen()
	instance, ok := a.pages.instances.Selected(a.installedList)
	if wasDetail && !a.pages.instances.DetailOpen() {
		a.resetInstanceContent()
		a.closeResources()
	}
	if ok && a.pages.instances.DetailOpen() {
		a.inspectInstance(instance, false)
		kind := a.pages.instances.ResourceKind()
		if kind.Valid() && (!a.resources.data.Open || !a.resources.data.Embedded || a.resources.instance.Key() != instance.Key() || a.resources.instance.GameVersion != instance.GameVersion || a.resources.instance.Loader != instance.Loader || a.resources.data.Kind != kind) {
			a.openInstanceResources(instance.Key(), kind, true)
		}
	}
	if ok && a.pages.instances.ServersOpen() {
		a.inspectServers(instance, false)
	} else {
		a.releaseServers()
	}
	a.servers.data.Locked = a.serversBusy()
	a.resources.data.Busy = a.tasksFull() || a.pathBusy || a.instanceInUse(a.resources.instance.Key())
	activity := a.instanceActivity(instance.Key())
	active := ""
	if activity.Active {
		active = instance.Key()
	}
	a.pages.instances.View(c, page.InstancesData{
		ViewMode:           &a.cfg.Appearance.LibraryView,
		Statistics:         a.libraryStatistics(),
		Space:              a.installSpace.data,
		Loaders:            a.loaderCatalog.data,
		Installed:          a.installedList,
		Versions:           a.versions,
		ManifestLoading:    a.manifestLoading,
		ManifestError:      a.manifestError,
		InstallDirectories: a.installDirectories(),
		Busy:               a.tasksFull() || a.pathBusy,
		InstanceBusy:       a.instanceInUse(instance.Key()),
		Activity:           a.instanceActivity,
		InUse:              a.instanceInUse,
		ContentRoot:        func(v minecraft.Instance) string { return v.ContentDirectory(a.cfg.GameForInstance(v.Key()).Isolate) },
		ActiveInstance:     active,
		Status:             activity.Status,
		Icons:              a.icons.bitmaps,
		Settings:           a.cfg.Instances,
		Content:            a.instanceContent.data,
		Resources:          a.resourceViewData(),
		Servers:            &a.servers.data,
	}, page.InstancesActions{
		New:           func() { a.openInstall("") },
		ViewChanged:   a.saveConfig,
		AssessInstall: a.assessInstall,
		Reorder:       a.reorderInstance,
		Install:       a.install,
		Loaders:       a.loadLoaderVersions,
		Repair:        a.repair,
		Refresh: func() {
			a.scanInstalled()
			if instance, ok := a.pages.instances.Selected(a.installedList); ok && a.pages.instances.DetailOpen() {
				a.inspectInstance(instance, true)
			}
		},
		Open:       a.openDirectory,
		OpenFile:   a.openFile,
		Settings:   a.openInstanceSettings,
		Edit:       a.openInstanceEdit,
		Components: a.openComponents,
		Copy:       a.openInstanceCopy,
		Shortcut:   a.openInstanceShortcut,
		CopyLink: func(key string) {
			services.WriteClipboard(launchrequest.Link(key))
			a.notify(locales.LinkCopied(), "", false)
		},
		Export:        a.openPackExport,
		UpdatePack:    a.openPackUpdate,
		AssociatePack: a.openPackAssociate,
		RestorePack:   a.openPackRestore,
		Import:        a.openPackImport,
		BrowsePacks:   a.openPackBrowser,
		RefreshContent: func() {
			if a.pages.instances.ServersOpen() {
				a.refreshServers()
				return
			}
			if a.resources.data.Embedded && !a.pages.instances.ResourceKind().Valid() {
				a.closeResources()
			}
			if instance, ok := a.pages.instances.Selected(a.installedList); ok {
				a.inspectInstance(instance, true)
			}
		},
		Content: func(kind resource.Kind) {
			if instance, ok := a.pages.instances.Selected(a.installedList); ok {
				a.openInstanceResources(instance.Key(), kind, true)
			}
		},
		ResourceActions: a.resourceActions(),
		LoadThumbnail:   a.loadContentThumbnail,
		LoadIcon:        a.loadInstanceIcon,
		LoadWorld:       a.loadWorldMetadata,
		EditWorld:       a.openWorldEdit,
		WorldArchive:    a.openWorldArchive,
		WorldPacks:      a.openWorldPacks,
		Preview:         a.previewScreenshot,
		ClosePreview:    a.closeScreenshotPreview,
		EditScreenshot:  a.openScreenshotEdit,
		Leave: func() {
			a.resetInstanceContent()
			a.closeResources()
			a.releaseServers()
		},
		ServerActions: a.serverActions(),
	})
}

func (a *app) accountPage(c *ui.Context) {
	a.dialogs.accounts.View(c, a.accountData(), a.accountActions(c.Services()))
}

func (a *app) accountData() page.AccountData {
	return page.AccountData{
		Accounts: a.cfg.Accounts, TopNavigation: true,
		Locked: a.preparingGames() || a.pathBusy, Busy: a.authBusy,
		Avatar: func(id string) *ui.Bitmap { return a.avatars[id].bitmap },
		Code:   a.authCode, URL: a.authURL, Status: a.authStatus, Error: a.accountError,
		ExternalServer: a.externalAuth.server, ExternalProfiles: a.externalAuth.session.Profiles,
	}
}

func (a *app) accountActions(services ui.Services) page.AccountActions {
	return page.AccountActions{
		AddOffline: a.addOffline, Login: func() { a.loginMicrosoft(services) }, Select: a.selectAccount,
		Remove: a.removeAccount, Cancel: a.cancelAuth,
		Appearance:    a.openAppearance,
		ExternalBegin: a.beginExternalAuth, ExternalInspect: a.inspectExternalServer, ExternalLogin: a.loginExternal,
		ExternalSelect: a.selectExternalProfile, ExternalClose: a.closeExternalAuth,
		ExternalBack: a.backExternalAuth,
		SaveServer:   a.saveAuthServer, RemoveServer: a.removeAuthServer,
		ClearError: func() { a.accountError = "" },
	}
}

func (a *app) gamePage(c *ui.Context) {
	a.pages.game.View(c, a.cfg, a.memory, a.memoryError, a.saveConfig)
}

func (a *app) javaPage(c *ui.Context) {
	page.Java(c, a.cfg, page.JavaData{
		Java:        a.java,
		JavaLoading: a.javaLoading,
		RescanJava:  a.rescanJava,
		BrowseJava:  func() { a.browseJava(false) },
		AddJava:     a.addJava,
	}, a.saveConfig)
}

func (a *app) generalPage(c *ui.Context) {
	counts := make(map[string]int)
	for _, v := range a.installedList {
		counts[v.SourceDir]++
	}
	page.General(c, page.GeneralData{
		Language:        &a.cfg.Appearance.Language,
		Directories:     a.cfg.Directories,
		DirectoryCounts: counts,
		DirectoryLocked: a.hasTasks() || a.pathBusy || len(a.games) > 0,
		Storage:         a.store.Format(),
		ConfigPath:      a.store.Path(),
		StorageChanging: a.storageChanging,
		StorageError:    a.storageError,
		Logging:         &a.cfg.Logging,
		LogError:        a.logLimitError,
		Launcher:        &a.cfg.Launcher,
	}, page.GeneralActions{
		LanguageChanged: func() { mygo.App.SetLocale(a.cfg.Appearance.Language); a.saveConfig() },
		Migrate:         a.changeStorage,
		LogLimitChanged: a.logLimitChanged,
		LauncherChanged: a.saveConfig,
		Directories: page.DirectoryActions{
			Add: a.addDirectories, Edit: a.editDirectory, Remove: a.removeDirectory, Open: a.openDirectory,
		},
	})
}

func (a *app) downloadPage(c *ui.Context) {
	if a.downloadCategory == "" {
		a.downloadCategory = string(resource.Mod)
	}
	if a.downloadCategory == downloadCategoryPack && !a.packBrowser.data.Open && !a.dialogs.pack.data.Open {
		a.openDownloadPackBrowser()
	}
	if kind, ok := downloadResourceKind(a.downloadCategory); ok {
		a.ensureDownloadResources()
		a.resources.data.Kind = kind
	}
	r := &a.resources
	r.data.Busy = a.tasksFull() || a.pathBusy
	ui.Column(c).Fill().MinHeight(0).Gap(kit.S12).Children(func() {
		resourceDetail := r.data.Project != nil && a.downloadCategory != downloadCategoryPack && a.downloadCategory != downloadCategoryGame
		packDetail := a.downloadCategory == downloadCategoryPack && a.packBrowser.data.Project != nil
		if !resourceDetail && !packDetail {
			a.downloadHeader(c)
		}
		c.Key("download-content-" + a.downloadCategory)
		kit.ContentMotion(c, ui.Column(c).Grow(1).MinHeight(0)).Children(func() {
			switch a.downloadCategory {
			case downloadCategoryGame:
				a.downloadVersions(c)
			case downloadCategoryPack:
				a.downloadPacks(c)
			default:
				if len(a.installedList) == 0 {
					a.downloadNoTarget(c)
				} else {
					a.pages.resources.View(c, a.resourceViewData(), a.resourceActions())
				}
			}
		})
	})
}

func (a *app) networkPage(c *ui.Context) {
	a.refreshCacheUsage(false)
	c.After(cacheUsageInterval)
	cacheError := a.cacheError
	if cacheError == "" && a.cacheUsage.err != nil {
		cacheError = a.cacheUsage.err.Error()
	}
	a.pages.download.View(c, a.cfg, page.DownloadData{
		VersionCount:   len(a.versions),
		InstalledCount: len(a.installed),
		CacheBusy:      a.hasTasks() || a.pathBusy || a.manifestLoading,
		CacheError:     cacheError,
		CacheStatus:    a.cacheStatus,
		CacheSize:      a.cacheUsage.bytes,
		CacheSizeKnown: a.cacheUsage.known,
	}, page.DownloadActions{
		SourceChanged: a.sourceChanged,
		Reload:        a.loadManifest,
		Save:          a.saveConfig,
		ChangeCache:   a.changeCacheDirectory,
		ClearCache:    a.clearCache,
		OpenCache:     func() { a.openDirectory(a.cfg.CacheDirectory()) },
	})
}

func (a *app) ensureDownloadResources() {
	r := &a.resources
	kind, ok := downloadResourceKind(a.downloadCategory)
	if !ok || r.data.FullPage && r.data.Open {
		return
	}
	// The downloads page has no target instance until one is chosen at install
	// time, so browsing starts unconstrained.
	r.request++
	r.instance = minecraft.Instance{}
	r.data = page.ResourcesData{ID: r.request, Open: true, FullPage: true, Kind: kind, Online: true, Source: resourcecatalog.CurseForge, Compatibility: resourcecatalog.CompatibilityFilter{GameVersion: resourcecatalog.AnyCompatibility, Loader: resourcecatalog.AnyCompatibility}}
	a.searchResources("", 0)
}

func (a *app) resourceViewData() *page.ResourcesData {
	a.resources.data.Targets = a.installedList
	a.resources.data.GameVersions = a.versions
	return &a.resources.data
}

func (a *app) logsPage(c *ui.Context) {
	if !a.logHistory.initialized {
		a.refreshLogHistory()
	}
	var tabs func()
	if a.cfg.Appearance.Navigation != config.NavSidebar {
		tabs = func() {
			ui.Row(c).Gap(4).Children(func() {
				for _, item := range sections[a.section].items {
					c.Key(item.page)
					kit.NavTab(c, item.name(), a.page == item.page, func() { a.goPage(item.page) }) // nav:shell
				}
			})
		}
	}
	a.pages.logs.View(c, page.LogsData{Lines: a.logs, Start: a.logStart, Revision: a.logRevision, History: a.logHistory.data}, page.LogsActions{
		Tabs:  tabs,
		Clear: a.clearLogs, Refresh: a.refreshLogHistory, Choose: a.chooseLogHistory,
		Analyze:       a.openLogDiagnostic,
		OpenDirectory: func() { a.openDirectory(a.store.LogDir()) },
	})
}

func (a *app) appearancePage(c *ui.Context) {
	a.pages.appearance.View(c, &a.cfg.Appearance, a.fonts, a.zoomPercent(), a.maxZoom(), a.previewZoom, a.changeZoom, a.appearanceChanged, a.gpuChanged, a.fpsChanged)
}

func (a *app) aboutPage(c *ui.Context) {
	page.About(c, a.store.Path())
}
