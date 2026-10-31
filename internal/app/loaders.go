package app

import (
	"context"

	"github.com/UruhaLushia/tomori/internal/app/page"
)

type loaderCatalogState struct {
	id     uint64
	cancel context.CancelFunc
	data   page.LoaderCatalogData
}

func (a *app) loadLoaderVersions(game, kind string) {
	a.loadLoaderCatalog(&a.loaderCatalog, game, kind)
}

func (a *app) loadLoaderCatalog(state *loaderCatalogState, game, kind string) {
	if state.data.Game == game && state.data.Kind == kind && (state.data.Loading || state.data.Error == "") {
		return
	}
	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	state.id++
	token := state.id
	state.data = page.LoaderCatalogData{Game: game, Kind: kind, Loading: kind != ""}
	if kind == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	state.cancel = cancel
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		versions, err := service.LoaderVersions(ctx, game, kind)
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			if token != state.id || a.ctx.Err() != nil {
				return
			}
			state.data.Loading, state.cancel = false, nil
			if err != nil {
				state.data.Error = err.Error()
			} else {
				state.data.Versions = versions
			}
		})
	})
}
