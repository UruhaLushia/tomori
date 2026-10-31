package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) openResourceUpdates() {
	r := &a.resources
	if !r.data.Open || !a.resourceEditable() {
		return
	}
	r.data.UpdatesOpen = true
	if !r.data.UpdateSource.Valid() {
		r.data.UpdateSource = resourcecatalog.CurseForge
	}
	a.checkResourceUpdates()
}

func (a *app) closeResourceUpdates() {
	a.resourceSpace.close()
	r := &a.resources
	a.closeMissingResources()
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.request++
	r.data.UpdatesOpen, r.data.UpdatesLoading, r.data.Loading = false, false, false
	r.data.UpdateCheck, r.data.UpdateDetail = nil, nil
	r.data.UpdateDetails = resourcecatalog.VersionDetails{}
	r.data.UpdateError = ""
}

func (a *app) checkResourceUpdates() {
	r := &a.resources
	if !r.data.Open || !r.data.UpdatesOpen || !r.data.UpdateSource.Valid() {
		return
	}
	a.closeMissingResources()
	ctx, token := a.resourceContext(3 * time.Minute)
	cancel := r.cancel
	r.data.Loading, r.data.UpdatesLoading = false, true
	r.data.UpdateCheck, r.data.UpdateDetail, r.data.UpdateError = nil, nil, ""
	r.data.UpdateDetails = resourcecatalog.VersionDetails{}
	instance, kind, source := r.instance, r.data.Kind, r.data.UpdateSource
	world := r.data.World
	instance.GameVersion = r.data.GameVersion
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var result resourcecatalog.UpdateCheck
		var err error
		if world != "" {
			result, err = service.CheckWorldPackUpdates(ctx, instance, world, source)
		} else {
			result, err = service.CheckResourceUpdates(ctx, instance, kind, source)
		}
		a.win.Update(func() {
			r := &a.resources
			if token != r.request || !r.data.UpdatesOpen || a.ctx.Err() != nil {
				return
			}
			r.data.UpdatesLoading = false
			if err != nil {
				r.data.UpdateError = err.Error()
				return
			}
			r.data.UpdateCheck = &result
		})
	})
}

func (a *app) backResourceUpdates() {
	r := &a.resources
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.request++
	r.data.UpdateDetail, r.data.UpdateError = nil, ""
	r.data.UpdateDetails = resourcecatalog.VersionDetails{}
	r.data.UpdatesLoading, r.data.Loading = false, false
}

func (a *app) resourceUpdateDetails(item resourcecatalog.ResourceUpdate) {
	r := &a.resources
	if !r.data.UpdatesOpen || r.data.UpdateCheck == nil || !slices.ContainsFunc(r.data.UpdateCheck.Updates, func(other resourcecatalog.ResourceUpdate) bool {
		return other.SHA1 == item.SHA1 && sameResourceVersion(other.Version, item.Version)
	}) {
		return
	}
	ctx, token := a.resourceContext(time.Minute)
	cancel := r.cancel
	r.data.Loading, r.data.UpdatesLoading = false, true
	r.data.UpdateDetail, r.data.UpdateError = &item, ""
	r.data.UpdateDetails = resourcecatalog.VersionDetails{}
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		result, err := service.ResourceVersionDetails(ctx, item.Version)
		a.win.Update(func() {
			r := &a.resources
			if token != r.request || !r.data.UpdatesOpen || a.ctx.Err() != nil {
				return
			}
			r.data.UpdatesLoading = false
			if err != nil {
				r.data.UpdateError = err.Error()
				return
			}
			r.data.UpdateDetails = result
		})
	})
}

func (a *app) updateResources(items []resourcecatalog.ResourceUpdate) {
	r := &a.resources
	if !a.resourceEditable() || !r.data.UpdatesOpen || r.data.UpdateCheck == nil || len(items) == 0 {
		return
	}
	for _, item := range items {
		if item.Version.Source != r.data.UpdateSource || !slices.ContainsFunc(r.data.UpdateCheck.Updates, func(other resourcecatalog.ResourceUpdate) bool {
			return other.SHA1 == item.SHA1 && other.Entry.Relative == item.Entry.Relative && sameResourceVersion(other.Version, item.Version)
		}) {
			return
		}
	}
	items = slices.Clone(items)
	instance, root, kind, viewID := r.instance, r.data.Root, r.data.Kind, r.data.ID
	world := r.data.World
	instance.GameVersion = r.data.GameVersion
	ctx, service, token := a.beginOperation(locales.UpdateResources()+" · "+instance.Name, instance.Key(), root)
	if token == 0 {
		return
	}
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	r.request++
	r.data.UpdateError = ""
	a.closeMissingResources()
	progress := a.progressHandler(token)
	a.workers.Go(func() {
		defer service.Close()
		var count int
		var err error
		if world != "" {
			count, err = service.UpdateWorldPacks(ctx, instance, world, items, progress)
		} else {
			count, err = service.UpdateResources(ctx, instance, kind, items, progress)
		}
		a.win.Update(func() {
			if !a.operationCurrent(token) || a.ctx.Err() != nil {
				return
			}
			r := &a.resources
			if r.data.ID != viewID || r.data.Root != root || r.data.Kind != kind {
				return
			}
			if err != nil {
				if !r.data.UpdatesOpen {
					return
				}
				if !errors.Is(err, context.Canceled) {
					r.data.UpdateError = err.Error()
					var missing *resourcecatalog.MissingFilesError
					if errors.As(err, &missing) {
						a.showMissingResources(missing.Files, nil, items)
					}
				}
				return
			}
			a.closeResourceUpdates()
			r.data.Status = fmt.Sprintf(locales.UpdatedFiles(), count)
			if r.data.Open && !r.data.Online {
				a.refreshResources()
			}
		})
		a.finishOperation(token, err)
	})
}

func sameResourceVersion(a, b resourcecatalog.Version) bool {
	return a.Source == b.Source && a.ID == b.ID && a.ProjectID == b.ProjectID && a.Number == b.Number
}
