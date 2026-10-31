package app

import (
	"runtime"
	"runtime/debug"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
)

func (a *app) createMainWindow() {
	a.win = mygo.NewWindow(mygo.WindowOptions{
		Title:           "Tomori",
		Width:           1180,
		Height:          760,
		MinWidth:        minimumWindowWidth,
		MinHeight:       minimumWindowHeight,
		BackgroundColor: kit.HexColor(kit.Window),
		Frameless:       true,
		StateKey:        "main",
		Content:         ui.View(a.view),
	})
	a.refreshZoomScreen()
	a.win.OnMove(a.refreshZoomScreen)
	a.win.OnBlur(a.cancelZoomOutline)
	a.win.OnHide(a.cancelZoomOutline)
	a.win.OnResize(a.cancelZoomOutline)
	offDisplays := mygo.Screen.OnDisplaysChanged(a.refreshZoomScreen)
	a.win.OnClose(a.launcherClosing)
	a.win.OnShow(a.launcherShown)
	a.win.OnRestore(a.launcherShown)
	a.win.OnClosed(func() {
		offDisplays()
		if a.recreatingWindow {
			a.closeViews()
			a.destroyTray()
			a.recreatingWindow = false
			a.win = nil
			runtime.GC()
			debug.FreeOSMemory()
			a.createMainWindow()
			return
		}
		a.closeViews()
		a.destroyTray()
		a.shutdown()
	})
	a.win.OnFileDrop(func(event *mygo.FileDropEvent) {
		a.dropFiles(event.Paths)
	})
	if a.cfg.Appearance.ShowFPS {
		a.openFPSWindow()
	}
}

func (a *app) recreateMainWindow() {
	if a.win == nil || a.recreatingWindow || a.ctx.Err() != nil {
		return
	}
	a.recreatingWindow = true
	a.win.Destroy()
}
