package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type componentsState struct {
	data      page.ComponentsData
	request   uint64
	cancel    context.CancelFunc
	catalog   loaderCatalogState
	operation uint64
}

func (a *app) openComponents(key string) {
	if a.tasksFull() || a.pathBusy || a.instanceInUse(key) {
		return
	}
	if a.offerPackRecovery(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok {
		return
	}
	a.pages.components = page.Components{}
	a.components.data = page.ComponentsData{Open: true, Key: key, Name: instance.Name}
	a.pathBusy = true
	a.inspectComponents()
}

func (a *app) inspectComponents() {
	s := &a.components
	if !s.data.Open || s.data.Saving {
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
	token := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.data.Loading, s.data.Error = true, ""
	a.workers.Go(func() {
		defer cancel()
		components, err := minecraft.InspectComponents(ctx, instance)
		a.win.Update(func() {
			if a.ctx.Err() != nil || token != a.components.request || !a.components.data.Open {
				return
			}
			d := &a.components.data
			d.Loading = false
			if err != nil {
				d.Error = err.Error()
				return
			}
			d.Current, d.Game = components, components.Game
		})
	})
}

func (a *app) closeComponents() {
	s := &a.components
	if s.data.Saving {
		a.cancelOperation(s.operation)
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.catalog.cancel != nil {
		s.catalog.cancel()
		s.catalog.cancel = nil
	}
	s.catalog.id++
	s.catalog.data = page.LoaderCatalogData{}
	s.request++
	s.data = page.ComponentsData{}
	a.pages.components = page.Components{}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) applyComponents(game string, kind modpack.Loader) {
	s := &a.components
	d := &s.data
	if !d.Open || d.Loading || d.Saving || a.tasksFull() || d.Current.Reason != "" || a.instanceInUse(d.Key) {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		return
	}
	expected, request := d.Current.Fingerprint, s.request
	ctx, service, token := a.beginOperation(locales.ChangingInstanceComponents()+instance.Name, d.Key, instance.ContentDirectory(a.cfg.GameForInstance(d.Key).Isolate))
	if token == 0 {
		return
	}
	d.Saving, d.Error = true, ""
	s.operation = token
	progress := a.progressHandler(token)
	a.workers.Go(func() {
		defer service.Close()
		err := service.ChangeComponents(ctx, instance, expected, game, kind, progress, func(line string) { a.logf("%s", line) })
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			if a.components.request == request && a.components.data.Open {
				a.components.data.Saving = false
				if err != nil {
					a.components.data.Error = err.Error()
				} else {
					a.closeComponents()
				}
			}
			if err == nil {
				a.scanInstalled()
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) componentsView(c *ui.Context) {
	a.components.data.Versions = a.versions
	a.components.data.Loaders = a.components.catalog.data
	a.pages.components.View(c, &a.components.data, page.ComponentsActions{
		Close: a.closeComponents, Retry: a.inspectComponents,
		Manifest: a.loadManifest,
		Loaders:  func(game, kind string) { a.loadLoaderCatalog(&a.components.catalog, game, kind) },
		Save:     a.applyComponents,
	})
}
