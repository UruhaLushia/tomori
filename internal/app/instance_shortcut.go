package app

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"

	"github.com/UruhaLushia/tomori/internal/shortcut"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) openInstanceShortcut(key string) {
	if a.pathBusy || a.shortcutBusy {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	directory, _ := mygo.App.Path(mygo.PathDesktop)
	a.openDirectoryPicker(pickerShortcut, directory)
	a.directoryPicker.instanceKey = key
}

func (a *app) createInstanceShortcut(key, directory string) {
	instance, ok := a.instanceByKey(key)
	if !ok || a.shortcutBusy || !instance.CanLaunch() {
		return
	}
	executable := os.Getenv("APPIMAGE")
	if runtime.GOOS != "linux" || !filepath.IsAbs(executable) {
		var err error
		executable, err = os.Executable()
		if err != nil {
			a.notifyError(locales.CreateShortcut(), err)
			return
		}
	}
	a.shortcutBusy = true
	budget := a.downloadBudget
	a.workers.Go(func() {
		release, err := budget.FilePermit(a.ctx)
		var path string
		if err == nil {
			path, err = shortcut.Create(a.ctx, directory, instance.Name, executable, instance.Key())
			release()
		}
		a.win.Update(func() {
			a.shortcutBusy = false
			if a.ctx.Err() != nil {
				return
			}
			if err != nil {
				a.notifyError(locales.CreateShortcut(), err)
			} else {
				a.notify(locales.ShortcutCreated(), path, false)
			}
		})
	})
}
