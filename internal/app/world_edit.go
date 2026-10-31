package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type worldEditState struct {
	data               page.WorldEditData
	request, operation uint64
}

func (a *app) openWorldEdit(file instancecontent.File, mode string) {
	if a.pathBusy || a.tasksFull() || mode != "rename" && mode != "copy" && mode != "delete" {
		return
	}
	key := a.instanceContent.data.Key
	instance, ok := a.instanceByKey(key)
	if !ok || a.instanceInUse(key) {
		return
	}
	folder := filepath.Base(file.Path)
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	if filepath.Clean(file.Path) != filepath.Join(root, "saves", folder) {
		return
	}
	found := false
	for _, candidate := range a.instanceContent.data.Summary.Worlds {
		if candidate.Path == file.Path {
			found = true
			break
		}
	}
	if !found {
		return
	}
	name := file.Name
	if world := a.instanceContent.data.Worlds[file.Path]; world.Name != "" {
		name = world.Name
	}
	if mode == "copy" {
		name = fmt.Sprintf(locales.Copy2(), name)
	}
	d := page.WorldEditData{Open: true, Mode: mode, Key: key, Folder: folder, Name: name, Source: instance.Name, Path: file.Path, Target: key}
	if mode == "copy" {
		for _, target := range a.installedList {
			if !target.CanLaunch() || a.instanceInUse(target.Key()) {
				continue
			}
			label := target.Name
			if target.SourceName != "" {
				label += " · " + target.SourceName
			}
			d.Targets = append(d.Targets, kit.SelectOption[string]{Value: target.Key(), Label: label})
		}
	}
	a.worldEdit.request++
	a.worldEdit.data = d
	a.pathBusy = true
}

func (a *app) closeWorldEdit() {
	s := &a.worldEdit
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data, s.operation = page.WorldEditData{}, 0
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) saveWorldEdit() {
	s := &a.worldEdit
	if !s.data.Open || s.data.Busy || a.tasksFull() {
		return
	}
	d := s.data
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	if a.instanceInUse(d.Key) {
		s.data.Error = locales.InstanceIsInUse()
		return
	}
	root := instance.ContentDirectory(a.cfg.GameForInstance(d.Key).Isolate)
	if filepath.Join(root, "saves", d.Folder) != d.Path {
		s.data.Error = locales.WorldDirectoryChangedRefreshAndTryAgain()
		return
	}
	request := launcher.WorldChange{Action: d.Mode, Folder: d.Folder, Name: d.Name, Target: instance}
	if d.Mode == "copy" {
		request.Target, ok = a.instanceByKey(d.Target)
		if !ok || !request.Target.CanLaunch() {
			s.data.Error = locales.InvalidTargetInstance()
			return
		}
		if a.instanceInUse(d.Target) {
			s.data.Error = locales.InstanceIsInUse()
			return
		}
	}
	label := locales.RenameWorld()
	switch d.Mode {
	case "copy":
		label = locales.CopyWorld()
	case "delete":
		label = locales.DeleteWorld()
	}
	ctx, token := a.beginTask(label+" · "+d.Source, d.Key, root)
	if token == 0 {
		return
	}
	if d.Mode == "copy" {
		a.operations[token].target = request.Target.ContentDirectory(a.cfg.GameForInstance(d.Target).Isolate)
		a.operations[token].visible = true
	}
	service, viewID := a.newService(), s.request
	s.data.Busy, s.data.Error, s.operation = true, "", token
	a.workers.Go(func() {
		defer service.Close()
		_, err := service.ChangeWorld(ctx, instance, request, func(p minecraft.Progress) {
			a.win.Update(func() {
				if a.ctx.Err() != nil || !a.operationCurrent(token) {
					return
				}
				job := &a.operations[token].job.data
				if !job.Canceling {
					job.Status = stageLabel(p)
				}
				job.Done, job.Total, job.FilesDone = p.Done, p.Total, p.FilesDone
				if a.worldEdit.request != viewID || !a.worldEdit.data.Open {
					return
				}
				d := &a.worldEdit.data
				d.Bytes, d.TotalBytes, d.Files = p.Done, p.Total, p.FilesDone
			})
		}, mygo.Shell.TrashItem)
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			current := a.worldEdit.request == viewID && a.worldEdit.data.Open
			if current {
				a.worldEdit.data.Busy = false
				if err != nil && !errors.Is(err, context.Canceled) {
					a.worldEdit.data.Error = err.Error()
				}
			}
			if err != nil {
				return
			}
			if current {
				a.closeWorldEdit()
			}
			if a.page == pageInstances && a.pages.instances.DetailOpen() {
				selected, ok := a.pages.instances.Selected(a.installedList)
				if ok && (selected.Key() == d.Key || selected.Key() == d.Target) {
					a.releaseInstanceMedia()
					a.inspectInstance(selected, true)
				}
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) worldEditView(c *ui.Context) {
	page.WorldEdit(c, &a.worldEdit.data, a.saveWorldEdit, a.closeWorldEdit)
}
