package app

import (
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type screenshotEditState struct {
	data               page.ScreenshotEditData
	request, operation uint64
}

func (a *app) openScreenshotEdit(file instancecontent.File, mode string) {
	if a.pathBusy || a.tasksFull() || a.instanceContent.data.Loading || mode != "export" && mode != "delete" {
		return
	}
	key := a.instanceContent.data.Key
	instance, ok := a.instanceByKey(key)
	if !ok {
		return
	}
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	if a.instanceContent.data.Root != root || filepath.Clean(file.Path) != filepath.Join(root, "screenshots", file.Name) {
		return
	}
	found := false
	for _, candidate := range a.instanceContent.data.Summary.Screenshots {
		if candidate.Path == file.Path {
			file, found = candidate, true
			break
		}
	}
	if !found {
		return
	}
	directory, _ := mygo.App.Path(mygo.PathDownloads)
	if directory == "" {
		directory, _ = os.UserHomeDir()
	}
	a.closeScreenshotPreview()
	s := &a.screenshotEdit
	s.request++
	s.data = page.ScreenshotEditData{Open: true, Mode: mode, Key: key, Root: root, Directory: directory, File: file}
	a.pathBusy = true
}

func (a *app) closeScreenshotEdit() {
	s := &a.screenshotEdit
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data, s.operation = page.ScreenshotEditData{}, 0
	if a.directoryPicker.data.Open && a.directoryPicker.kind == pickerScreenshotExport {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) chooseScreenshotDirectory(directory string, request uint64) {
	s := &a.screenshotEdit
	if s.data.Open && !s.data.Busy && s.request == request {
		s.data.Directory, s.data.Error = directory, ""
	}
}

func (a *app) saveScreenshotEdit() {
	s := &a.screenshotEdit
	d := s.data
	if !d.Open || d.Busy || d.Result != "" || a.tasksFull() || d.Mode == "export" && d.Directory == "" {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok || instance.ContentDirectory(a.cfg.GameForInstance(d.Key).Isolate) != d.Root {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	label := locales.ExportScreenshot()
	if d.Mode == "delete" {
		label = locales.DeleteScreenshot()
	}
	ctx, token := a.beginTask(label+" · "+d.File.Name, d.Key, d.Root)
	if token == 0 {
		return
	}
	service, viewID := a.newService(), s.request
	s.data.Busy, s.data.Error, s.operation = true, "", token
	a.workers.Go(func() {
		defer service.Close()
		var result string
		var err error
		if d.Mode == "delete" {
			err = service.TrashScreenshot(ctx, instance, d.File, mygo.Shell.TrashItem)
		} else {
			result, err = service.ExportScreenshot(ctx, instance, d.File, d.Directory)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			if s.request == viewID && s.data.Open {
				s.data.Busy = false
				if err != nil {
					if !operationCanceled(err) {
						s.data.Error = err.Error()
					}
				} else if d.Mode == "delete" {
					a.closeScreenshotEdit()
				} else {
					s.data.Result = result
				}
			}
			if err == nil && d.Mode == "delete" && a.instanceContent.data.Key == d.Key && a.instanceContent.data.Root == d.Root {
				a.releaseInstanceMedia()
				a.inspectInstance(instance, true)
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) screenshotEditView(c *ui.Context) {
	s := &a.screenshotEdit
	if !s.data.Open {
		return
	}
	page.ScreenshotEdit(c, &s.data, page.ScreenshotEditActions{Close: a.closeScreenshotEdit, Save: a.saveScreenshotEdit, Browse: func() {
		if s.data.Open && !s.data.Busy {
			a.openDirectoryPicker(pickerScreenshotExport, s.data.Directory)
		}
	}, Open: func() {
		if s.data.Result != "" {
			a.openDirectory(filepath.Dir(s.data.Result))
		}
	}})
}
