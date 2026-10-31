package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type instanceCopyState struct {
	data               page.InstanceCopyData
	request, operation uint64
}

func (a *app) openInstanceCopy(key string) {
	if a.pathBusy || a.tasksFull() || a.instanceInUse(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	a.instanceCopy.request++
	a.instanceCopy.data = page.InstanceCopyData{Open: true, Key: key, Source: instance.Name, Settings: true, Request: launcher.InstanceCopy{Name: fmt.Sprintf(locales.Copy2(), instance.Name), Directory: config.DefaultGameDirectory()}}
	a.pathBusy = true
}

func (a *app) closeInstanceCopy() {
	s := &a.instanceCopy
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.operation = 0
	s.data = page.InstanceCopyData{}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) copyInstance() {
	s := &a.instanceCopy
	if !s.data.Open || s.data.Busy || a.tasksFull() || a.instanceInUse(s.data.Key) {
		return
	}
	instance, ok := a.instanceByKey(s.data.Key)
	if !ok {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	request, viewID := s.data.Request, s.request
	ctx, token := a.beginTask(locales.CopyInstance(), instance.Key(), instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate))
	if token == 0 {
		return
	}
	service := a.newService()
	s.data.Busy, s.data.Error, s.operation = true, "", token
	settings, hasSettings := a.cfg.Instances[instance.Key()]
	hasSettings = hasSettings && s.data.Settings
	selected := a.currentVersion()
	a.workers.Go(func() {
		defer service.Close()
		copy, err := service.CopyInstance(ctx, instance, request, func(p minecraft.Progress) {
			a.win.Update(func() {
				if a.ctx.Err() != nil || !a.operationCurrent(token) || a.instanceCopy.request != viewID || !a.instanceCopy.data.Open {
					return
				}
				d := &a.instanceCopy.data
				d.Bytes, d.TotalBytes, d.Files, d.TotalFiles = p.Done, p.Total, p.FilesDone, p.Files
			})
		})
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			current := a.instanceCopy.request == viewID && a.instanceCopy.data.Open
			if current {
				a.instanceCopy.data.Busy = false
				if err != nil && !errors.Is(err, context.Canceled) {
					a.instanceCopy.data.Error = err.Error()
				}
			}
			if err != nil {
				return
			}
			if hasSettings {
				if a.cfg.Instances == nil {
					a.cfg.Instances = make(map[string]config.InstanceSettings)
				}
				a.cfg.Instances[copy.Key()] = settings
			}
			if current && a.currentVersion() == selected {
				a.cfg.Directories.SelectedInstance = copy.Key()
			}
			a.saveConfig()
			if current {
				a.closeInstanceCopy()
			}
			a.scanInstalled()
		})
		a.finishOperation(token, err)
	})
}

func (a *app) instanceCopyView(c *ui.Context) {
	a.instanceCopy.data.Directories = a.installDirectories()
	page.InstanceCopy(c, &a.instanceCopy.data, a.copyInstance, a.closeInstanceCopy)
}
