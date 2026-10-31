package app

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) closeMissingResources() {
	r := &a.resources
	if r.supplementTask != 0 {
		a.cancelOperation(r.supplementTask)
		r.supplementTask = 0
	}
	r.missingID++
	r.retryVersion, r.retryUpdates = nil, nil
	r.retryPolicy = resource.InstallDefault
	r.data.Missing, r.data.MissingOpen = nil, false
	r.data.MissingError, r.data.MissingStatus = "", ""
	r.data.MissingBusy = false
}

func (a *app) showMissingResources(files []resourcecatalog.MissingFile, version *resourcecatalog.Version, updates []resourcecatalog.ResourceUpdate, policy ...resource.InstallPolicy) {
	a.closeMissingResources()
	r := &a.resources
	r.data.Missing, r.data.MissingOpen = slices.Clone(files), true
	r.data.Error, r.data.UpdateError = "", ""
	if version != nil {
		copy := *version
		r.retryVersion = &copy
		if len(policy) > 0 {
			r.retryPolicy = policy[0]
		}
	}
	r.retryUpdates = slices.Clone(updates)
}

func (a *app) browseResourceSupplement() {
	if !a.resourceEditable() || !a.resources.data.MissingOpen || len(a.resources.data.Missing) == 0 {
		return
	}
	a.openDirectoryPicker(pickerResourceFiles, "")
}

func (a *app) supplementResourcePaths(paths []string) {
	r := &a.resources
	if !a.resourceEditable() || !r.data.MissingOpen || len(r.data.Missing) == 0 || len(paths) == 0 {
		return
	}
	paths = slices.Clone(paths)
	files := slices.Clone(r.data.Missing)
	missingID, viewID := r.missingID, r.data.ID
	ctx, token := a.beginTask(locales.AddMissingResourceFiles(), r.instance.Key(), r.data.Root)
	if token == 0 {
		return
	}
	service := a.newService()
	r.supplementTask, r.data.MissingBusy = token, true
	r.data.MissingError, r.data.MissingStatus = "", ""
	a.workers.Go(func() {
		defer service.Close()
		count, err := service.SupplementResources(ctx, files, paths)
		a.win.Update(func() {
			if !a.operationCurrent(token) || a.ctx.Err() != nil {
				return
			}
			r := &a.resources
			if r.missingID != missingID || r.data.ID != viewID || !r.data.MissingOpen {
				return
			}
			r.supplementTask, r.data.MissingBusy = 0, false
			if count > 0 {
				r.data.MissingStatus = fmt.Sprintf(locales.AddedFiles(), count)
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				r.data.MissingError = err.Error()
			}
		})
		a.finishOperation(token, err)
	})
}

func (a *app) retryMissingResources() {
	r := &a.resources
	if !r.data.MissingOpen || !a.resourceEditable() {
		return
	}
	if len(r.retryUpdates) > 0 {
		a.updateResources(slices.Clone(r.retryUpdates))
		return
	}
	if r.retryVersion != nil {
		a.installResource(*r.retryVersion, r.retryPolicy, "")
	}
}
