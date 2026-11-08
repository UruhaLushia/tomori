package app

import (
	"context"
	"errors"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
)

type packRestoreState struct {
	data               page.PackRestoreData
	view               page.PackRestore
	request, operation uint64
	cancel             context.CancelFunc
	confirm            chan struct{}
	instance           minecraft.Instance
	pending            *launcher.PackRecovery
}

func (a *app) openPackRestore(key string) {
	if a.pathBusy || a.tasksFull() || a.instanceInUse(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	a.closePackRestore()
	a.packRestore.instance = instance
	a.packRestore.data = page.PackRestoreData{Open: true, Key: key, Name: instance.Name}
	a.pathBusy = true
	a.loadPackBackups()
}

func (a *app) closePackRestore() {
	s := &a.packRestore
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.operation != 0 {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.operation = 0
	s.data = page.PackRestoreData{}
	s.view = page.PackRestore{}
	s.confirm = nil
	s.instance, s.pending = minecraft.Instance{}, nil
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) loadPackBackups() {
	s := &a.packRestore
	if !s.data.Open || s.operation != 0 {
		return
	}
	instance, pending := s.instance, s.pending
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	id := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.data.Busy, s.data.Error, s.data.Status = true, "", locales.Loading()
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var backups []modpack.BackupInfo
		var err error
		if pending != nil {
			var item launcher.PackRecovery
			var found bool
			item, found, err = service.PendingPackRecovery(ctx, pending.Info.Name+".json")
			if found {
				backups = []modpack.BackupInfo{item.Info}
			}
		} else {
			backups, err = service.PackBackups(ctx, instance)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.cancel = nil
			s.data.Busy = false
			s.data.Backups = backups
			if err != nil && !operationCanceled(err) {
				s.data.Error = err.Error()
			}
		})
	})
}

func (a *app) inspectPackBackup(info modpack.BackupInfo) {
	s := &a.packRestore
	if !s.data.Open || s.data.Busy || s.data.Ready || s.operation != 0 || info.Issue != "" || a.tasksFull() || a.instanceInUse(s.data.Key) {
		return
	}
	instance, pending := s.instance, s.pending
	content := instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate)
	if pending != nil {
		content = pending.Content
	}
	ctx, token := a.beginTask(locales.RestoreModpack()+" · "+instance.Name, instance.Key(), content)
	if token == 0 {
		return
	}
	s.operation, s.cancel = token, a.operations[token].cancel
	confirm := make(chan struct{}, 1)
	s.confirm = confirm
	s.data.Busy, s.data.Error, s.data.Status = true, "", locales.CheckingBackup()
	id := s.request
	service := a.newService()
	progress := func(p minecraft.Progress) {
		label := stageLabel(p)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open || !a.operationCurrent(token) {
				return
			}
			s.data.Status = label
		})
	}
	a.workers.Go(func() {
		defer service.Close()
		var prepared *launcher.PackRestore
		var err error
		if pending != nil {
			prepared, err = service.PreparePendingPackRestore(ctx, *pending, progress)
		} else {
			prepared, err = service.PreparePackRestore(ctx, instance, info, progress)
		}
		backup := ""
		if err == nil {
			report, e := prepared.Plan.AssessSpace(ctx)
			err = e
			if err == nil {
				a.win.Update(func() {
					if a.ctx.Err() != nil || ctx.Err() != nil || s.request != id || !s.data.Open {
						return
					}
					s.data.Info = prepared.Info
					s.data.Space = page.SpaceData{Report: report}
					s.data.Busy, s.data.Ready = false, true
				})
				select {
				case <-ctx.Done():
					err = ctx.Err()
				case <-confirm:
					backup, err = service.ApplyPackRestore(ctx, prepared, progress)
				}
			}
			err = errors.Join(err, prepared.Plan.Close())
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			if s.request == id && s.data.Open {
				s.cancel, s.confirm, s.operation = nil, nil, 0
				s.data.Busy, s.data.Ready = false, false
				s.data.Backup = backup
				if err != nil && !operationCanceled(err) {
					s.data.Error = err.Error()
				}
			}
			if backup != "" {
				a.scanInstalled()
			}
			a.loadPackRecoveries(false)
		})
		a.finishOperation(token, err)
	})
}

func (a *app) applyPackRestore() {
	s := &a.packRestore
	if !s.data.Open || s.data.Busy || !s.data.Ready || s.data.Space.Blocked() || !a.operationCurrent(s.operation) {
		return
	}
	s.data.Busy, s.data.Status = true, locales.RestoreModpack()
	select {
	case s.confirm <- struct{}{}:
	default:
	}
}

func (a *app) backPackRestore() {
	s := &a.packRestore
	if !s.data.Open || s.data.Busy || !s.data.Ready {
		return
	}
	s.data.Busy, s.data.Status = true, locales.Canceling()
	a.cancelOperation(s.operation)
}

func (a *app) packRestoreView(c *ui.Context) {
	s := &a.packRestore
	s.view.View(c, &s.data, page.PackRestoreActions{
		Close: a.closePackRestore, Refresh: a.loadPackBackups, Choose: a.inspectPackBackup, Apply: a.applyPackRestore, Back: a.backPackRestore,
		OpenBackup: func() {
			if s.data.Backup != "" {
				a.openDirectory(s.data.Backup)
			}
		},
	})
}
