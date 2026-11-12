package app

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type resourceImportState struct {
	data    page.ResourceImportData
	request uint64
	cancel  context.CancelFunc
}

func (a *app) closeResourceImport() {
	s := &a.resourceImport
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.request++
	s.data = page.ResourceImportData{}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) resourceImportView(c *ui.Context) {
	s := &a.resourceImport
	page.ResourceImport(c, &s.data, page.ResourceImportActions{
		Cancel:         a.closeResourceImport,
		SelectInstance: a.selectResourceImportInstance,
		SelectWorld:    a.selectResourceImportWorld,
	})
}

func (a *app) startResourceImport(paths []string) {
	if a.tasksFull() || a.pathBusy || len(paths) == 0 || len(paths) > 1000 {
		return
	}
	paths = slices.Clone(paths)
	sort.Strings(paths)
	paths = slices.Compact(paths)
	a.closeResourceImport()
	s := &a.resourceImport
	s.request++
	token := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.data = page.ResourceImportData{Open: true, Loading: true}
	a.pathBusy = true
	selected := ""
	if a.page == pageInstances && a.pages.instances.CanGoBack() {
		if instance, ok := a.pages.instances.Selected(a.installedList); ok {
			selected = instance.Key()
		}
	}
	a.workers.Go(func() {
		files := make([]resource.LocalFile, 0, len(paths))
		var problems []error
		for _, path := range paths {
			kind, err := resource.Detect(ctx, path)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s: %w", filepath.Base(path), err))
				continue
			}
			files = append(files, resource.LocalFile{Path: path, Kind: kind})
		}
		a.win.Update(func() {
			if token != s.request || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.cancel = nil
			s.data.Loading = false
			if len(problems) > 0 {
				s.data.Error = errors.Join(problems...).Error()
				return
			}
			if slices.ContainsFunc(files, func(file resource.LocalFile) bool { return file.Kind == "" }) {
				if len(files) == 1 && isPackFile(files[0].Path) {
					a.closeResourceImport()
					a.inspectPack(files[0].Path)
					return
				}
				s.data.Error = locales.UnsupportedFileType()
				return
			}
			s.data.Files = files
			if selected != "" {
				a.selectResourceImportInstance(selected)
				return
			}
			s.data.Instances = slices.DeleteFunc(slices.Clone(a.installedList), func(instance minecraft.Instance) bool { return !instance.CanLaunch() })
			s.data.Icons = a.icons.bitmaps
		})
	})
}

func (a *app) selectResourceImportInstance(key string) {
	s := &a.resourceImport
	if !s.data.Open || s.data.Loading {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		s.data.Error = locales.InvalidTargetInstance()
		return
	}
	if a.instanceInUse(key) {
		s.data.Error = locales.InstanceIsInUse()
		return
	}
	s.data.Target = key
	if !slices.ContainsFunc(s.data.Files, func(file resource.LocalFile) bool { return file.Kind == resource.DataPack }) {
		a.beginResourceImport(instance, "")
		return
	}
	s.request++
	token := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	s.data.Loading = true
	root := instance.ContentDirectory(a.cfg.GameForInstance(key).Isolate)
	a.workers.Go(func() {
		summary, err := instancecontent.Scan(ctx, root)
		a.win.Update(func() {
			if token != s.request || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.cancel = nil
			s.data.Loading = false
			if err != nil {
				s.data.Error = err.Error()
				return
			}
			s.data.Worlds = summary.Worlds
			if len(s.data.Worlds) == 0 {
				s.data.Error = locales.NoWorlds()
			}
		})
	})
}

func (a *app) selectResourceImportWorld(path string) {
	s := &a.resourceImport
	if !s.data.Open || s.data.Loading || s.data.Target == "" {
		return
	}
	for _, world := range s.data.Worlds {
		if world.Path == path {
			s.data.World = filepath.Base(path)
			instance, ok := a.instanceByKey(s.data.Target)
			if ok {
				a.beginResourceImport(instance, s.data.World)
			}
			return
		}
	}
}

func (a *app) beginResourceImport(instance minecraft.Instance, world string) {
	s := &a.resourceImport
	files := slices.Clone(s.data.Files)
	kinds := make([]string, 0, len(files))
	for _, file := range files {
		label := file.Kind.Label()
		if !slices.Contains(kinds, label) {
			kinds = append(kinds, label)
		}
	}
	a.closeResourceImport()
	ctx, service, token := a.beginOperation(locales.Import()+" · "+strings.Join(kinds, "、")+" · "+instance.Name, "resources:"+instance.Key(), "")
	if token == 0 {
		return
	}
	a.workers.Go(func() {
		defer service.Close()
		count, err := service.ImportLocalResources(ctx, instance, files, world, a.progressHandler(token))
		a.win.Update(func() {
			if !a.operationCurrent(token) || a.ctx.Err() != nil {
				return
			}
			if err == nil && count > 0 {
				title := fmt.Sprintf(locales.Imported2(), count, strings.Join(kinds, "、"))
				a.operations[token].silent = true
				a.notify(title, instance.Name, false)
				a.logf("%s · %s", title, instance.Name)
			}
			if a.instanceContent.data.Key == instance.Key() {
				a.inspectInstance(instance, true)
			}
			if a.resources.data.Open && a.resources.instance.Key() == instance.Key() {
				a.refreshResources()
			}
		})
		a.finishOperation(token, err)
	})
}
