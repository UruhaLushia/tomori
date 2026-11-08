package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type packExportState struct {
	data               page.PackExportData
	request, operation uint64
}

func (a *app) openPackExport(key string) {
	if a.pathBusy || a.tasksFull() || a.instanceInUse(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	directory, _ := mygo.App.Path(mygo.PathDownloads)
	if directory == "" {
		directory, _ = os.UserHomeDir()
	}
	version := instance.PackVersion
	if version == "" {
		version = "1.0.0"
	}
	a.packExport.request++
	a.packExport.data = page.PackExportData{Open: true, Key: key, Source: instance.Name, Request: launcher.PackExport{Name: instance.Name, Version: version, Description: instance.Description, Format: "mrpack", Directory: directory, Filename: launcher.ExportFilename(instance.Name), References: true}}
	a.pathBusy = true
}

func (a *app) closePackExport() {
	s := &a.packExport
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.operation = 0
	s.data = page.PackExportData{}
	if a.directoryPicker.data.Open && a.directoryPicker.kind == pickerPackExport {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) exportPack() {
	s := &a.packExport
	if !s.data.Open || s.data.Busy || s.data.Path != "" || a.tasksFull() || a.instanceInUse(s.data.Key) {
		return
	}
	instance, ok := a.instanceByKey(s.data.Key)
	if !ok {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	request, viewID := s.data.Request, s.request
	ctx, token := a.beginTask(locales.ExportModpack(), instance.Key(), instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate))
	if token == 0 {
		return
	}
	s.data.Busy, s.data.Error, s.operation = true, "", token
	service := a.newService()
	a.workers.Go(func() {
		defer service.Close()
		result, err := service.ExportPack(ctx, instance, request, func(p minecraft.Progress) {
			a.win.Update(func() {
				if a.ctx.Err() != nil || !a.operationCurrent(token) || a.packExport.request != viewID || !a.packExport.data.Open {
					return
				}
				d := &a.packExport.data
				d.Bytes, d.Files, d.Identifying = p.Done, p.FilesDone, p.Stage == "export-identify"
			})
		})
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) || a.packExport.request != viewID || !a.packExport.data.Open {
				return
			}
			d := &a.packExport.data
			d.Busy = false
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					d.Error = err.Error()
				}
				return
			}
			d.Path = result.Path
		})
		a.finishOperation(token, err)
	})
}

func (a *app) packExportView(c *ui.Context) {
	page.PackExport(c, &a.packExport.data, page.PackExportActions{Save: a.exportPack, Close: a.closePackExport, Browse: func() {
		if a.packExport.data.Open && !a.packExport.data.Busy {
			a.openDirectoryPicker(pickerPackExport, a.packExport.data.Request.Directory)
		}
	}, Open: func() {
		if a.packExport.data.Path != "" {
			a.openDirectory(filepath.Dir(a.packExport.data.Path))
		}
	}})
}

func (a *app) pathDialogsOpen() bool {
	return a.directoryPicker.data.Open || a.directoryEditor.data.Open || a.instanceEdit.data.Open || a.instanceCopy.data.Open || a.worldEdit.data.Open || a.worldArchive.data.Open || a.worldBackups.data.Open || a.screenshotEdit.data.Open || a.servers.data.Edit.Open || a.appearance.data.Open || a.logDiagnostic.data.Open || a.packExport.data.Open || a.components.data.Open || a.packUpdate.data.Open || a.packAssociate.data.Open || a.packRestore.data.Open || a.packRecovery.data.Open
}

func (a *app) chooseExportDirectory(directory string, request uint64) {
	if !a.packExport.data.Open || a.packExport.data.Busy || a.packExport.request != request {
		return
	}
	a.packExport.data.Request.Directory = filepath.Clean(directory)
	a.packExport.data.Error = ""
}
