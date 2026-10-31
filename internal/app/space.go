package app

import (
	"context"
	"slices"
	"time"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/internal/storage"
)

type spaceAssessment struct {
	key    string
	id     uint64
	cancel context.CancelFunc
	data   page.SpaceData
}

func (s *spaceAssessment) close() {
	s.id++
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.key, s.data = nil, "", page.SpaceData{}
}

func (a *app) assessSpace(state *spaceAssessment, key string, assess func(context.Context, *launcher.Service) (storage.Report, error)) {
	if state.key == key {
		return
	}
	state.close()
	if key == "" || a.ctx.Err() != nil {
		return
	}
	state.key, state.data.Loading = key, true
	ctx, cancel := context.WithTimeout(a.ctx, 25*time.Second)
	state.cancel = cancel
	id := state.id
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		timer := time.NewTimer(250 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		report, err := assess(ctx, service)
		a.win.Update(func() {
			if a.ctx.Err() != nil || state.id != id {
				return
			}
			state.data = page.SpaceData{Report: report}
			if err != nil {
				state.data.Error = err.Error()
			}
		})
	})
}

func (a *app) assessInstall(id, directory string, kind modpack.Loader) page.SpaceData {
	if id == "" {
		if a.installSpace.key != "" {
			a.installSpace.close()
		}
		return a.installSpace.data
	}
	summary, ok := a.summaryByID(id)
	if !ok || directory == "" {
		return page.SpaceData{}
	}
	key := id + "\x00" + directory + "\x00" + kind.ID + "\x00" + kind.Version
	a.assessSpace(&a.installSpace, key, func(ctx context.Context, service *launcher.Service) (storage.Report, error) {
		return service.AssessInstall(ctx, directory, summary, kind)
	})
	return a.installSpace.data
}

func (a *app) assessPackSpace() {
	p := &a.dialogs.pack
	if !p.data.Open || p.data.Info.Format == "" || p.data.Directory == "" {
		return
	}
	info, directory, optional := p.data.Info, p.data.Directory, p.data.Optional
	info = info.WithOptional(p.data.SelectedOptional)
	selection := ""
	for _, file := range info.Files {
		if file.Optional {
			optional = true
			selection += "\x00" + file.SelectionKey()
		}
	}
	preview := p.preview
	if preview != nil {
		selected := *preview
		selected.Info = info
		preview = &selected
	}
	key := p.archive.SHA512 + "\x00" + directory
	key += selection
	if preview != nil {
		key += "\x00" + preview.Version.URL
	}
	if optional {
		key += "\x00optional"
	}
	a.assessSpace(&a.packSpace, key, func(ctx context.Context, service *launcher.Service) (storage.Report, error) {
		if preview != nil {
			return service.AssessRemotePackImport(ctx, directory, *preview, optional)
		}
		return service.AssessPackImport(ctx, directory, info, optional)
	})
}

func (a *app) assessResourceSpace(key string, items []resourcecatalog.ResourceUpdate) page.SpaceData {
	if a.resourceSpace.key == key {
		return a.resourceSpace.data
	}
	if key == "" || len(items) == 0 {
		if a.resourceSpace.key != "" {
			a.resourceSpace.close()
		}
		return a.resourceSpace.data
	}
	items = slices.Clone(items)
	instance, kind := a.resources.instance, a.resources.data.Kind
	world := a.resources.data.World
	a.assessSpace(&a.resourceSpace, key, func(ctx context.Context, service *launcher.Service) (storage.Report, error) {
		if world != "" {
			return service.AssessWorldPackUpdates(ctx, instance, world, items)
		}
		return service.AssessResourceUpdates(ctx, instance, kind, items)
	})
	return a.resourceSpace.data
}
