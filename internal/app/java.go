package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) browseJava(instance bool) {
	if a.pathBusy || instance && a.instanceInUse(a.instanceSettings.Key) {
		return
	}
	kind, path := pickerJava, a.cfg.Java.Path
	if instance {
		if !a.instanceSettings.Open || !a.instanceSettings.Enabled {
			return
		}
		kind, path = pickerInstanceJava, a.instanceSettings.Config.Java.Path
	}
	if path == "" && len(a.java) > 0 {
		path = a.java[0].Path
	}
	if path != "" {
		path = filepath.Dir(path)
	} else {
		path, _ = os.UserHomeDir()
	}
	a.openDirectoryPicker(kind, path)
}

func (a *app) addJava() {
	if a.pathBusy {
		return
	}
	path := a.cfg.Java.Path
	if path == "" && len(a.java) > 0 {
		path = a.java[0].Path
	}
	if path != "" {
		path = filepath.Dir(path)
	} else {
		path, _ = os.UserHomeDir()
	}
	a.openDirectoryPicker(pickerJavaAdd, path)
}

func (a *app) confirmJavaFile(ctx context.Context, token uint64, path string) {
	p := &a.directoryPicker
	instance, manual := p.kind == pickerInstanceJava, p.kind == pickerJavaAdd
	key := p.instanceKey
	p.data.Saving, p.data.Error = true, ""
	win := p.win
	a.workers.Go(func() {
		info, err := java.InspectExecutable(ctx, path)
		win.Update(func() {
			if !a.pickerCurrent(ctx, token) {
				return
			}
			p := &a.directoryPicker
			p.data.Saving = false
			if err != nil {
				p.data.Error = err.Error()
				return
			}
			if instance && (!a.instanceSettings.Open || !a.instanceSettings.Enabled || a.instanceSettings.Key != key) {
				p.data.Error = locales.InstanceSettingsClosed()
				return
			}
			a.javaID++
			a.javaLoading = false
			if index := slices.IndexFunc(a.java, func(runtime java.Runtime) bool { return runtime.Path == info.Path }); index >= 0 {
				a.java[index] = info
			} else {
				a.java = append(a.java, info)
			}
			a.closeDirectoryPicker()
			if instance {
				a.instanceSettings.Config.Java.Path = info.Path
				a.instanceSettings.Config.Java.Mode = "custom"
				a.saveInstanceSettings()
			} else if manual {
				if !slices.Contains(a.cfg.Java.ManualPaths, info.Path) {
					a.cfg.Java.ManualPaths = append(a.cfg.Java.ManualPaths, info.Path)
				}
				a.saveConfig()
			} else {
				a.cfg.Java.Path = info.Path
				a.cfg.Java.Mode = "custom"
				a.saveConfig()
			}
			a.win.Invalidate()
		})
	})
}

func (a *app) rescanJava() {
	a.javaID++
	token := a.javaID
	a.javaLoading = true
	custom, dir := a.cfg.Java.Path, a.store.RuntimeDir()
	a.workers.Go(func() {
		list := java.Discover(a.ctx, custom, a.cfg.Java.ManualPaths...)
		list = append(list, java.Managed(a.ctx, dir)...)
		a.win.Update(func() {
			if token != a.javaID || a.ctx.Err() != nil {
				return
			}
			a.java, a.javaLoading = list, false
			a.drainLaunchRequests()
		})
	})
}
