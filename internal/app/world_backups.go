package app

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type worldBackupsState struct {
	data               page.WorldBackupsData
	view               page.WorldBackups
	request, operation uint64
	cancel             context.CancelFunc
}

func (a *app) openWorldBackups(file instancecontent.File) {
	key := a.instanceContent.data.Key
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() || a.pathBusy || a.tasksFull() || a.instanceInUse(key) {
		return
	}
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	d := page.WorldBackupsData{Open: true, Key: key, Root: root, Source: instance.Name, Mode: "new"}
	for _, world := range a.instanceContent.data.Summary.Worlds {
		folder := filepath.Base(world.Path)
		if filepath.Clean(world.Path) != filepath.Join(root, "saves", folder) {
			continue
		}
		d.Targets = append(d.Targets, kit.SelectOption[string]{Value: folder, Label: world.Name})
		if file.Path == world.Path {
			d.Target = folder
		}
	}
	if d.Target == "" && len(d.Targets) > 0 {
		d.Target = d.Targets[0].Value
	}
	a.worldBackups.data = d
	a.pathBusy = true
	a.loadWorldBackups()
}

func (a *app) closeWorldBackups() {
	s := &a.worldBackups
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data, s.view, s.operation = page.WorldBackupsData{}, page.WorldBackups{}, 0
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) loadWorldBackups() {
	s := &a.worldBackups
	if !s.data.Open || s.data.Busy {
		return
	}
	instance, ok := a.instanceByKey(s.data.Key)
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.request++
	id, service := s.request, a.newService()
	s.data.Ready, s.data.Delete, s.data.Loading = false, false, true
	s.data.Result, s.data.Backup, s.data.Error = "", "", ""
	s.data.Info, s.data.TargetInfo = launcher.WorldSource{}, launcher.WorldSource{}
	s.data.Selected = launcher.WorldBackup{}
	a.workers.Go(func() {
		defer service.Close()
		defer cancel()
		items, err := service.WorldBackups(ctx, instance)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.data.Loading = false
			s.data.Backups = items
			if err != nil && !operationCanceled(err) {
				s.data.Error = err.Error()
			}
		})
	})
}

func (a *app) chooseWorldBackup(item launcher.WorldBackup) {
	s := &a.worldBackups
	if !s.data.Open || s.data.Busy {
		return
	}
	for _, backup := range s.data.Backups {
		if backup.Name == item.Name {
			s.data.Selected = backup
			s.data.Info = launcher.WorldSource{}
			s.data.Ready, s.data.Delete, s.data.Mode, s.data.Name = true, false, "new", ""
			a.inspectWorldBackup()
			return
		}
	}
}

func (a *app) inspectWorldBackup() {
	s := &a.worldBackups
	if !s.data.Open || !s.data.Ready || s.data.Busy || s.data.Delete {
		return
	}
	d := s.data
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.request++
	id, service := s.request, a.newService()
	s.data.Loading, s.data.Error = true, ""
	s.data.TargetInfo, s.data.Space = launcher.WorldSource{}, page.SpaceData{}
	a.workers.Go(func() {
		defer service.Close()
		defer cancel()
		info, err := d.Info, error(nil)
		if info.Path == "" {
			info, err = service.InspectWorldBackup(ctx, instance, d.Selected)
		}
		var target launcher.WorldSource
		space := page.SpaceData{}
		if err == nil && d.Mode == "replace" {
			found := false
			for _, option := range d.Targets {
				if option.Value == d.Target {
					found = true
					break
				}
			}
			if found {
				target, err = service.InspectWorld(ctx, filepath.Join(d.Root, "saves", d.Target))
			}
		}
		if err == nil {
			if d.Mode == "replace" {
				space.Report, err = service.AssessWorldRestore(ctx, instance, info, target)
			} else {
				space.Report, err = service.AssessWorldImport(ctx, instance, info)
			}
			if err != nil {
				space.Error, err = err.Error(), nil
			}
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.data.Loading = false
			if err != nil {
				if !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				return
			}
			s.data.Info, s.data.TargetInfo, s.data.Space = info, target, space
			if s.data.Name == "" {
				s.data.Name = info.World.Name
				if s.data.Name == "" {
					s.data.Name = strings.TrimSuffix(d.Selected.Name, filepath.Ext(d.Selected.Name))
				}
			}
		})
	})
}

func (a *app) backWorldBackups() {
	s := &a.worldBackups
	if !s.data.Open || s.data.Busy {
		return
	}
	if s.data.Result != "" {
		a.loadWorldBackups()
		return
	}
	if s.data.Delete && s.data.Info.Path != "" {
		s.data.Delete = false
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.request++
	s.data.Ready, s.data.Delete, s.data.Loading = false, false, false
	s.data.Info, s.data.TargetInfo = launcher.WorldSource{}, launcher.WorldSource{}
	s.data.Result, s.data.Backup, s.data.Error = "", "", ""
}

func (a *app) askWorldBackupDelete(item launcher.WorldBackup) {
	s := &a.worldBackups
	if !s.data.Open || s.data.Busy {
		return
	}
	for _, backup := range s.data.Backups {
		if backup.Name == item.Name {
			if s.cancel != nil {
				s.cancel()
				s.cancel = nil
			}
			s.request++
			if s.data.Selected.Name != backup.Name {
				s.data.Info = launcher.WorldSource{}
			}
			s.data.Selected = backup
			s.data.Ready, s.data.Delete, s.data.Loading, s.data.Error = true, true, false, ""
			return
		}
	}
}

func (a *app) saveWorldBackup() {
	s := &a.worldBackups
	d := s.data
	if !d.Open || !d.Ready || d.Loading || d.Busy || d.Result != "" || a.tasksFull() || !d.Delete && (d.Info.Path == "" || d.Space.Blocked() || d.Mode == "replace" && d.TargetInfo.Path == "") {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok || instance.ContentDirectory(a.cfg.GameForInstance(d.Key).Isolate) != d.Root {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	if a.instanceInUse(d.Key) {
		s.data.Error = locales.InstanceIsInUse()
		return
	}
	label := locales.RestoreWorld()
	if d.Delete {
		label = locales.DeleteBackup()
	}
	ctx, token := a.beginTask(label+" · "+d.Source, d.Key, d.Root)
	if token == 0 {
		return
	}
	a.operations[token].visible = !d.Delete
	id, service := s.request, a.newService()
	s.data.Busy, s.data.Error, s.operation = true, "", token
	s.data.Done, s.data.Total, s.data.Files = 0, 0, 0
	a.workers.Go(func() {
		defer service.Close()
		progress := func(p minecraft.Progress) {
			a.win.Update(func() {
				if a.ctx.Err() != nil || !a.operationCurrent(token) {
					return
				}
				job := &a.operations[token].job.data
				if !job.Canceling {
					job.Status = stageLabel(p)
				}
				job.Done, job.Total, job.FilesDone = p.Done, p.Total, p.FilesDone
				if s.request == id && s.data.Open {
					s.data.Done, s.data.Total, s.data.Files = p.Done, p.Total, p.FilesDone
				}
			})
		}
		result := launcher.WorldRestoreResult{}
		var err error
		if d.Delete {
			err = service.TrashWorldBackup(ctx, instance, d.Selected, mygo.Shell.TrashItem)
		} else if d.Mode == "replace" {
			result, err = service.RestoreWorld(ctx, instance, d.Info, d.TargetInfo, progress)
		} else {
			result.Path, err = service.ImportWorld(ctx, instance, d.Info, d.Name, progress)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			if s.request == id && s.data.Open {
				s.data.Busy = false
				s.data.Result, s.data.Backup = result.Path, result.Backup
				if result.Path != "" && d.Mode == "new" {
					s.data.Targets = append(s.data.Targets, kit.SelectOption[string]{Value: filepath.Base(result.Path), Label: d.Name})
				}
				if err != nil && !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				if d.Delete && err == nil {
					a.loadWorldBackups()
				}
			}
			if result.Path != "" && a.page == pageInstances && a.pages.instances.DetailOpen() {
				selected, ok := a.pages.instances.Selected(a.installedList)
				if ok && selected.Key() == d.Key {
					a.releaseInstanceMedia()
					a.inspectInstance(selected, true)
				}
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) worldBackupsView(c *ui.Context) {
	s := &a.worldBackups
	if !s.data.Open {
		return
	}
	s.view.View(c, &s.data, page.WorldBackupsActions{Close: a.closeWorldBackups, Refresh: a.loadWorldBackups, Back: a.backWorldBackups, Changed: a.inspectWorldBackup, Save: a.saveWorldBackup, Choose: a.chooseWorldBackup, Delete: a.askWorldBackupDelete, Open: func() { a.openDirectory(filepath.Join(s.data.Root, "backups", "worlds")) }, OpenResult: func() {
		if s.data.Result != "" {
			a.openDirectory(s.data.Result)
		}
	}})
}
