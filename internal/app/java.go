package app

import (
	"context"
	"fmt"
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

func (a *app) downloadJava(request java.DownloadRequest) {
	if request.Distribution == "" {
		request.Distribution = java.DistributionMojang
	}
	if request.PackageType == "" {
		request.PackageType = java.PackageJRE
	}
	distribution, validDistribution := java.DistributionInfoFor(request.Distribution)
	valid := validDistribution && slices.Contains(java.SupportedMajors(), request.Major) && request.PackageType == java.PackageJDK
	if validDistribution && distribution.HasJRE {
		valid = valid || request.PackageType == java.PackageJRE
	}
	if !valid || a.pathBusy || a.javaDownloading || a.tasksFull() {
		return
	}
	a.javaDownloadRequest = request
	a.javaDownloadOpen = false
	distributionName, packageType := distribution.Label, "JRE"
	if request.PackageType == java.PackageJDK {
		packageType = "JDK"
	}
	title := fmt.Sprintf("%s %s %s %d", locales.DownloadJava(), distributionName, packageType, request.Major)
	key := fmt.Sprintf("java:%s:%s:%d", request.Distribution, request.PackageType, request.Major)
	ctx, service, token := a.beginOperation(title, key, a.store.RuntimeDir())
	if token == 0 {
		return
	}
	a.javaDownloading = true
	progress := a.progressHandler(token)
	a.workers.Go(func() {
		defer service.Close()
		_, err := service.DownloadJava(ctx, request, progress)
		a.win.Update(func() {
			a.javaDownloading = false
			if err == nil && a.ctx.Err() == nil {
				a.rescanJava()
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) deleteJava(runtime java.Runtime) {
	if a.pathBusy || a.javaDownloading || a.hasTasks() || a.preparingGames() {
		return
	}
	dir := a.store.RuntimeDir()
	if java.ManagedRoot(dir, runtime.Path) == "" {
		return
	}
	ctx, service, token := a.beginOperation(fmt.Sprintf("%s %s", locales.Delete(), runtime.Label()), "java-delete:"+runtime.Path, dir)
	if token == 0 {
		return
	}
	a.operations[token].silent = true
	a.workers.Go(func() {
		defer service.Close()
		err := java.RemoveManaged(ctx, dir, runtime.Path)
		a.win.Update(func() {
			if err == nil {
				if a.cfg.Java.Path == runtime.Path {
					a.cfg.Java.Path, a.cfg.Java.Mode = "", "auto"
				}
				for major, path := range a.cfg.Java.Versions {
					if path == runtime.Path {
						delete(a.cfg.Java.Versions, major)
					}
				}
				a.saveConfig()
				a.rescanJava()
			}
		})
		a.finishOperation(token, err)
	})
}
