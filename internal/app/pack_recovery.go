package app

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type packRecoveryState struct {
	data    page.PackRecoveryData
	view    page.PackRecovery
	request uint64
	cancel  context.CancelFunc
}

func (a *app) loadPackRecoveries(notify bool) {
	s := &a.packRecovery
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	id := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel, s.data.Busy = cancel, true
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		entries, err := service.PendingPackRecoveries(ctx)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id {
				return
			}
			s.cancel, s.data.Busy, s.data.Error = nil, false, ""
			s.data.Entries = entries
			if err != nil && !operationCanceled(err) {
				s.data.Error = err.Error()
				a.logf("%s: %v", locales.InterruptedUpdates(), err)
			}
			if notify && (len(entries) > 0 || s.data.Error != "") {
				a.notices = append(a.notices, ui.Toast{ID: "pack-recovery", Title: locales.UpdateInterrupted(), Action: locales.View(), OnAction: a.openPackRecoveries, Type: string(kit.FeedbackInfo), Timeout: 7 * time.Second})
			}
			a.drainLaunchRequests()
		})
	})
}

func (a *app) openPackRecoveries() {
	if a.pathBusy || a.tasksFull() {
		return
	}
	a.packRecovery.data.Open = true
	a.pathBusy = true
	a.loadPackRecoveries(false)
}

func (a *app) closePackRecoveries() {
	a.packRecovery.data.Open = false
	a.packRecovery.view = page.PackRecovery{}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) openPendingPackRestore(item launcher.PackRecovery) {
	if item.Info.Issue != "" || a.tasksFull() || a.instancePathsInUse(item.Instance.Key(), item.Instance.InstanceDir, item.Content) {
		return
	}
	a.closePackRecoveries()
	a.closePackRestore()
	s := &a.packRestore
	s.instance, s.pending = item.Instance, &item
	s.data = page.PackRestoreData{Open: true, Key: item.Instance.Key(), Name: item.Instance.Name, Backups: []modpack.BackupInfo{item.Info}}
	a.pathBusy = true
	a.inspectPackBackup(item.Info)
}

func (a *app) offerPackRecovery(key string) bool {
	instance, ok := a.instanceByKey(key)
	if !ok {
		return false
	}
	content := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	for _, item := range a.packRecovery.data.Entries {
		if item.Instance.Key() == key || pathsOverlap(content, item.Content) {
			a.openPackRecoveries()
			return true
		}
	}
	return false
}

func (a *app) packRecoveryButton(c *ui.Context) {
	s := &a.packRecovery
	if len(s.data.Entries) == 0 && s.data.Error == "" {
		return
	}
	c.Key("pack-recovery-button")
	kit.Button(c, locales.Restore2(), a.openPackRecoveries).Label(fmt.Sprintf(locales.InterruptedUpdates2(), len(s.data.Entries)))
}

func (a *app) packRecoveryView(c *ui.Context) {
	s := &a.packRecovery
	s.view.View(c, &s.data, page.PackRecoveryActions{Close: a.closePackRecoveries, Refresh: func() { a.loadPackRecoveries(false) }, Choose: a.openPendingPackRestore, OpenDirectory: a.openDirectory})
}
