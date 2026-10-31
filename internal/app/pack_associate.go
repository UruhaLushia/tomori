package app

import (
	"context"
	"path/filepath"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type packAssociateState struct {
	data               page.PackAssociateData
	view               page.PackAssociate
	request, operation uint64
	cancel             context.CancelFunc
	version            packcatalog.Version
	preview            *launcher.PackPreview
	archive            modpack.ArchiveIdentity
}

func (a *app) openPackAssociate(key string) {
	if a.pathBusy || a.tasksFull() || a.instanceInUse(key) {
		return
	}
	if a.offerPackRecovery(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	a.closePackAssociate()
	a.packAssociate.data = page.PackAssociateData{Open: true, Key: key, Name: instance.Name}
	a.pathBusy = true
}

func (a *app) closePackAssociate() {
	s := &a.packAssociate
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.operation != 0 {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.operation = 0
	s.data = page.PackAssociateData{}
	s.view = page.PackAssociate{}
	s.version, s.preview, s.archive = packcatalog.Version{}, nil, modpack.ArchiveIdentity{}
	if a.directoryPicker.data.Open && a.directoryPicker.kind == pickerPackAssociate {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) changePackAssociation() {
	s := &a.packAssociate
	if !s.data.Open || s.data.Busy {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.request++
	s.view = page.PackAssociate{}
	s.data.Selected, s.data.Path, s.data.Error = "", "", ""
	s.data.Versions, s.data.Info = nil, modpack.Info{}
	s.data.Confirmed, s.data.Loading = false, false
	s.version, s.preview, s.archive = packcatalog.Version{}, nil, modpack.ArchiveIdentity{}
}

func (a *app) readPackReference() {
	s := &a.packAssociate
	if !s.data.Open || s.data.Busy || s.data.Loading {
		return
	}
	a.changePackAssociation()
	s.data.Loading = true
	id, link := s.request, s.data.URL
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	s.cancel = cancel
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		v, versions, err := service.ResolvePackReference(ctx, link)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.cancel, s.data.Loading = nil, false
			if err != nil {
				if !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				return
			}
			s.data.Versions = versions
			a.choosePackReference(v)
		})
	})
}

func (a *app) choosePackReference(v packcatalog.Version) {
	s := &a.packAssociate
	if !s.data.Open || s.data.Busy || s.data.Loading || !slices.ContainsFunc(s.data.Versions, func(item packcatalog.Version) bool {
		return item.Source == v.Source && item.ProjectID == v.ProjectID && item.ID == v.ID
	}) {
		return
	}
	s.version = v
	s.data.Selected, s.data.Path, s.data.Error = v.ID, "", ""
	s.data.Info, s.preview, s.archive = modpack.Info{}, nil, modpack.ArchiveIdentity{}
	s.data.Confirmed = false
	if v.URL != "" {
		a.inspectAssociation("")
	}
}

func (a *app) inspectAssociation(path string) {
	s := &a.packAssociate
	if !s.data.Open || s.data.Busy || s.data.Loading || s.version.ID == "" {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	id, v := s.request, s.version
	s.data.Loading, s.data.Confirmed = true, false
	s.data.Error, s.data.Info = "", modpack.Info{}
	s.preview, s.archive = nil, modpack.ArchiveIdentity{}
	s.data.Path = path
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	s.cancel = cancel
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var preview launcher.PackPreview
		var identity modpack.ArchiveIdentity
		var err error
		if path != "" {
			preview.Version = v
			preview.Info, identity, err = service.InspectPack(ctx, path)
		} else {
			preview, err = service.PreviewPack(ctx, v)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.cancel, s.data.Loading = nil, false
			if err != nil {
				if !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				return
			}
			s.preview, s.archive = &preview, identity
			s.data.Info = preview.Info
		})
	})
}

func (a *app) associatePack() {
	s := &a.packAssociate
	d := &s.data
	if !d.Open || d.Busy || d.Loading || !d.Confirmed || d.Info.Format == "" || a.tasksFull() || a.instanceInUse(d.Key) {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		d.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	request := launcher.PackAssociation{Version: s.version, Path: d.Path, Archive: s.archive, Preview: s.preview, Confirmed: d.Confirmed, Optional: d.Optional}
	ctx, token := a.beginTask(locales.LinkModpack()+" · "+instance.Name, instance.Key(), instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate))
	if token == 0 {
		return
	}
	s.operation = token
	id := s.request
	d.Busy, d.Error, d.Status = true, "", locales.ReadingModpack()
	service := a.newService()
	progress := a.progressHandler(token)
	a.workers.Go(func() {
		defer service.Close()
		_, err := service.AssociatePack(ctx, instance, request, func(p minecraft.Progress) {
			progress(p)
			if p.Download != nil {
				return
			}
			label := stageLabel(p)
			a.win.Update(func() {
				if a.ctx.Err() != nil || !a.operationCurrent(token) {
					return
				}
				a.operations[token].visible = p.Stage == "packfiles"
				if s.request == id && s.data.Open {
					s.data.Status = label
				}
			})
		})
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			if s.request == id && s.data.Open {
				s.data.Busy, s.operation = false, 0
				if err != nil {
					if !operationCanceled(err) {
						s.data.Error = err.Error()
					}
				} else {
					a.closePackAssociate()
				}
			}
			if err == nil {
				a.scanInstalled()
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) packAssociateView(c *ui.Context) {
	s := &a.packAssociate
	s.view.View(c, &s.data, page.PackAssociateActions{Close: a.closePackAssociate, Read: a.readPackReference, Changed: a.changePackAssociation, Choose: a.choosePackReference, Save: a.associatePack, Browse: func() {
		if s.data.Open && !s.data.Busy && !s.data.Loading && s.version.ID != "" {
			path := ""
			if s.data.Path != "" {
				path = filepath.Dir(s.data.Path)
			}
			a.openDirectoryPicker(pickerPackAssociate, path)
		}
	}})
}
