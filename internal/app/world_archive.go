package app

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type worldArchiveState struct {
	data               page.WorldArchiveData
	request, operation uint64
	cancel             context.CancelFunc
}

func (a *app) openWorldArchive(mode string, file instancecontent.File) {
	if mode == "backups" {
		a.openWorldBackups(file)
		return
	}
	if a.pathBusy || a.tasksFull() || mode != "backup" && mode != "file" && mode != "directory" && mode != "restore" {
		return
	}
	key := a.instanceContent.data.Key
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() || a.instanceInUse(key) {
		return
	}
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	if mode == "backup" {
		found := false
		for _, candidate := range a.instanceContent.data.Summary.Worlds {
			if candidate.Path == file.Path && filepath.Clean(file.Path) == filepath.Join(root, "saves", filepath.Base(file.Path)) {
				found = true
				break
			}
		}
		if !found {
			return
		}
	}
	a.worldArchive.request++
	a.worldArchive.data = page.WorldArchiveData{Open: true, Key: key, Mode: mode, Source: instance.Name, Root: root}
	a.pathBusy = true
	if mode == "backup" {
		a.inspectWorldArchive(file.Path)
	}
}

func (a *app) closeWorldArchive() {
	s := &a.worldArchive
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data, s.operation = page.WorldArchiveData{}, 0
	if a.directoryPicker.data.Open && (a.directoryPicker.kind == pickerWorldFile || a.directoryPicker.kind == pickerWorldDirectory) {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) inspectWorldArchive(path string) {
	s := &a.worldArchive
	if !s.data.Open || s.data.Busy || s.data.Loading && s.data.Path == path {
		return
	}
	instance, ok := a.instanceByKey(s.data.Key)
	if !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.data.Path, s.data.Error, s.data.Loading, s.data.Name = path, "", true, ""
	s.data.Info, s.data.Space = launcher.WorldSource{}, page.SpaceData{}
	viewID, mode, service := s.request, s.data.Mode, a.newService()
	a.workers.Go(func() {
		defer service.Close()
		defer cancel()
		info, err := service.InspectWorld(ctx, path)
		if err == nil && mode == "backup" && !info.Directory {
			err = errors.New(locales.InvalidWorldDirectory())
		}
		space := page.SpaceData{}
		if err == nil {
			if mode == "backup" {
				space.Report, err = service.AssessWorldBackup(ctx, instance, info)
			} else {
				space.Report, err = service.AssessWorldImport(ctx, instance, info)
			}
			if err != nil {
				space.Error, err = err.Error(), nil
			}
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.worldArchive.data.Open || a.worldArchive.request != viewID {
				return
			}
			d := &a.worldArchive.data
			d.Loading = false
			if err != nil {
				d.Error = err.Error()
				return
			}
			d.Info, d.Space, d.Name = info, space, info.World.Name
			if d.Mode != "backup" && d.Mode != "restore" {
				d.Mode = "file"
				if info.Directory {
					d.Mode = "directory"
				}
			}
			if d.Name == "" {
				d.Name = filepath.Base(info.Path)
			}
		})
	})
}

func (a *app) saveWorldArchive() {
	s := &a.worldArchive
	d := s.data
	if !d.Open || d.Busy || d.Loading || d.Result != "" || d.Info.Path == "" || d.Space.Blocked() || a.tasksFull() {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok || instance.ContentDirectory(a.cfg.GameForInstance(d.Key).Isolate) != d.Root {
		s.data.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	if a.instanceInUse(d.Key) || d.Info.Directory && a.instancePathsInUse("", d.Info.Path, d.Info.Path) {
		s.data.Error = locales.InstanceIsInUse()
		return
	}
	label := locales.ImportWorld()
	switch d.Mode {
	case "backup":
		label = locales.BackUpWorld()
	case "restore":
		label = locales.RestoreWorld()
	}
	ctx, service, token := a.beginOperation(label+" · "+d.Source, d.Key, d.Root)
	if token == 0 {
		return
	}
	if d.Info.Directory {
		a.operations[token].target = d.Info.Path
	}
	viewID := s.request
	s.data.Busy, s.data.Error, s.operation = true, "", token
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
				if a.worldArchive.request == viewID && a.worldArchive.data.Open {
					a.worldArchive.data.Done, a.worldArchive.data.Total, a.worldArchive.data.Files = p.Done, p.Total, p.FilesDone
				}
			})
		}
		var result string
		var err error
		if d.Mode == "backup" {
			result, err = service.BackupWorld(ctx, instance, filepath.Base(d.Info.Path), progress)
		} else {
			result, err = service.ImportWorld(ctx, instance, d.Info, d.Name, progress)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			current := a.worldArchive.request == viewID && a.worldArchive.data.Open
			if current {
				a.worldArchive.data.Busy = false
				if err != nil && !errors.Is(err, context.Canceled) {
					a.worldArchive.data.Error = err.Error()
				}
			}
			if err != nil {
				return
			}
			if d.Mode == "backup" {
				if current {
					a.worldArchive.data.Result = result
				}
				return
			}
			if current {
				a.closeWorldArchive()
			}
			if a.page == pageInstances && a.pages.instances.DetailOpen() {
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

func (a *app) dropWorldArchive(paths []string) {
	if !a.worldArchive.data.Open || a.worldArchive.data.Mode == "backup" || a.worldArchive.data.Busy {
		return
	}
	if len(paths) != 1 {
		a.worldArchive.data.Error = locales.ChooseOneWorldFolderOrZipFile()
		return
	}
	a.inspectWorldArchive(paths[0])
}

func (a *app) chooseWorldArchive(path string, request uint64) {
	if a.worldArchive.request == request && a.worldArchive.data.Open && !a.worldArchive.data.Busy {
		a.inspectWorldArchive(path)
	}
}

func (a *app) worldArchiveView(c *ui.Context) {
	page.WorldArchive(c, &a.worldArchive.data, page.WorldArchiveActions{Save: a.saveWorldArchive, Close: a.closeWorldArchive, Drop: a.dropWorldArchive, Open: func() {
		if a.worldArchive.data.Result != "" {
			a.openDirectory(filepath.Dir(a.worldArchive.data.Result))
		}
	}, Browse: func() {
		d := &a.worldArchive.data
		if !d.Open || d.Busy {
			return
		}
		kind, initial := pickerWorldFile, filepath.Dir(d.Path)
		if d.Mode == "directory" {
			kind, initial = pickerWorldDirectory, d.Path
		} else if d.Mode == "restore" && d.Path == "" {
			initial = filepath.Join(d.Root, "backups", "worlds")
		}
		if d.Path == "" && d.Mode != "restore" {
			initial = ""
		}
		a.openDirectoryPicker(kind, initial)
	}})
}
