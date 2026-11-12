package app

import (
	"bytes"
	"image/png"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) closeViews() {
	a.closeFPSWindow()
	a.cancelZoomOutline()
	a.cancelHomeScan()
	a.icons.release()
	a.closeExternalAuth()
	a.launchRequests = nil
	a.releaseLogHistory()
	a.installSpace.close()
	a.packSpace.close()
	a.resourceSpace.close()
	a.resetInstanceContent()
	a.releaseServers()
	a.closeAppearance()
	a.closePackImport()
	a.closeResources()
	a.closeResourceImport()
	a.closeDirectoryEditor()
	a.closeComponents()
	a.closeInstanceEdit()
	a.closeInstanceCopy()
	a.closeWorldEdit()
	a.closeWorldArchive()
	a.closeWorldBackups()
	a.closeScreenshotEdit()
	a.closePackExport()
	a.closePackUpdate()
	a.closePackAssociate()
	a.closePackRestore()
	a.closePackRecoveries()
	a.closeDirectoryPicker()
}

func (a *app) launcherClosing(e *mygo.CloseEvent) {
	if a.forceQuit || a.cfg.Launcher.Close != config.LauncherWaitForGames || !a.runningGames() {
		return
	}
	e.PreventDefault()
	a.waitingForGames = true
	for _, token := range a.operationOrder {
		a.cancelOperation(token)
	}
	if a.authCancel != nil {
		a.authCancel()
	}
	if a.manifestCancel != nil {
		a.manifestCancel()
	}
	if a.scanCancel != nil {
		a.scanCancel()
	}
	a.closeViews()
	a.hideLauncher()
}

func (a *app) runningGames() bool {
	for _, session := range a.games {
		if session.activity.Running {
			return true
		}
	}
	return false
}

func (a *app) gameStarted() {
	if a.win == nil || a.waitingForGames {
		return
	}
	switch a.cfg.Launcher.Visibility {
	case config.LauncherHide:
		a.hideLauncher()
	case config.LauncherMinimize:
		a.launcherVisibility = config.LauncherMinimize
		a.backgroundLauncher()
		a.win.Minimize()
	}
}

func (a *app) gameEnded() {
	if len(a.games) != 0 || a.win == nil {
		return
	}
	if a.waitingForGames {
		a.quitLauncher()
		return
	}
	if a.launcherVisibility != "" {
		a.showLauncher()
	}
}

func (a *app) hideLauncher() {
	a.ensureTray()
	a.launcherVisibility = config.LauncherHide
	a.backgroundLauncher()
	a.win.Hide()
}

func (a *app) backgroundLauncher() {
	a.cancelHomeScan()
	a.icons.release()
	a.downloadOpen = false
	a.discardCompletedOperations()
	a.pages.downloadTasks = page.DownloadTasks{}
	a.releaseInstanceMedia()
}

func (a *app) showLauncher() {
	if a.win == nil || a.ctx.Err() != nil {
		return
	}
	a.launcherShown()
	a.win.Restore()
	a.win.Show()
	a.win.Focus()
}

func (a *app) launcherShown() {
	a.launcherVisibility = ""
	a.waitingForGames = false
	a.destroyTray()
}

func (a *app) quitLauncher() {
	a.forceQuit, a.waitingForGames = true, false
	mygo.App.Quit()
}

func (a *app) ensureTray() {
	if a.tray != nil || a.trayFailed {
		return
	}
	icon := ui.Render(func(c *ui.Context) {
		ui.Box(c).Fill().Center().Children(func() { ui.Icon(c, kit.Cube).FontSize(20).TextColor(kit.AccentText) })
	}, 32, 32, 1)
	var data bytes.Buffer
	if err := png.Encode(&data, icon); err != nil {
		a.trayFailed = true
		return
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{ID: "tomori", Icon: data.Bytes(), IconIsTemplate: true, ToolTip: "Tomori"})
	if err != nil {
		a.trayFailed = true
		return
	}
	a.tray = tray
	a.tray.OnClick(a.showLauncher)
	a.updateTray()
}

func (a *app) updateTray() {
	if a.tray == nil {
		return
	}
	a.tray.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Label: locales.OpenTomori(), Click: func(*mygo.MenuItem, *mygo.Window) { a.showLauncher() }},
		{Label: locales.StopGamesAndQuit(), Click: func(*mygo.MenuItem, *mygo.Window) { a.quitLauncher() }},
	}))
}

func (a *app) destroyTray() {
	if a.tray != nil {
		a.tray.Destroy()
		a.tray = nil
	}
}
