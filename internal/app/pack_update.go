package app

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type packUpdateState struct {
	data                              page.PackUpdateData
	request, operation, spaceRevision uint64
	cancel                            context.CancelFunc
	preview                           *launcher.PackPreview
	archive                           modpack.ArchiveIdentity
	source                            minecraft.PackSource
	local                             []string
	ready                             *launcher.PackUpdate
	decisions                         chan map[string]modpack.UpdateChoice
}

func (a *app) openPackUpdate(key string) {
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
	a.closePackUpdate()
	s := &a.packUpdate
	s.data = page.PackUpdateData{Open: true, Loading: true, Key: key, Name: instance.Name, Mode: 1}
	a.pathBusy = true
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	s.cancel = cancel
	id := s.request
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		header, err := service.PackOrigin(ctx, instance)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			d := &s.data
			d.Loading = err == nil && header.Source.Provider != ""
			if !d.Loading {
				s.cancel = nil
			}
			if err != nil {
				d.Error = err.Error()
				return
			}
			d.HasOrigin, d.Origin, d.Optional = true, header, header.Optional
			if header.Source.Provider != "" {
				d.Mode = 0
			}
		})
		if err != nil || header.Source.Provider == "" {
			return
		}
		versions, versionErr := service.PackVersions(ctx, header.Source.Provider, header.Source.ProjectID, "")
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.cancel, s.data.Loading = nil, false
			s.data.Versions = versions
			if versionErr != nil && !operationCanceled(versionErr) {
				s.data.Error = versionErr.Error()
			}
		})
	})
}

func (a *app) closePackUpdate() {
	s := &a.packUpdate
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.operation != 0 {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data = page.PackUpdateData{}
	s.preview, s.ready, s.decisions = nil, nil, nil
	s.local = nil
	s.archive, s.source = modpack.ArchiveIdentity{}, minecraft.PackSource{}
	s.operation = 0
	a.packUpdateSpace.close()
	a.pages.packUpdate = page.PackUpdate{}
	if a.directoryPicker.data.Open && (a.directoryPicker.kind == pickerPackUpdate || a.directoryPicker.kind == pickerPackUpdateFiles) {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) changeUpdateSource() {
	s := &a.packUpdate
	if s.data.Busy {
		return
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.request++
	s.preview = nil
	s.source, s.archive = minecraft.PackSource{}, modpack.ArchiveIdentity{}
	s.data.Info, s.data.Selected, s.data.Error = modpack.Info{}, "", ""
	s.data.DifferentSource, s.data.AllowDifferentSource, s.data.Deferred = false, false, false
	s.data.Loading = false
	s.data.Missing, s.local = nil, nil
	s.data.LocalCount = 0
	a.packUpdateSpace.close()
}

func (a *app) inspectUpdatePack(path string, remote *packcatalog.Version) {
	s := &a.packUpdate
	if !s.data.Open || !s.data.HasOrigin || s.data.Busy || s.data.Plan != nil {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	id := s.request
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	s.cancel = cancel
	s.data.Info, s.data.Error, s.data.Loading = modpack.Info{}, "", true
	s.data.AllowDifferentSource = false
	s.preview = nil
	s.local, s.data.Missing = nil, nil
	s.data.LocalCount = 0
	s.data.Path = path
	if remote != nil {
		s.data.Selected = remote.ID
	}
	link := s.data.URL
	a.packUpdateSpace.close()
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var info modpack.Info
		var archive modpack.ArchiveIdentity
		var preview *launcher.PackPreview
		var source minecraft.PackSource
		var err error
		if path != "" {
			info, archive, err = service.InspectPack(ctx, path)
		} else {
			var v packcatalog.Version
			if remote != nil {
				v = *remote
			} else {
				v, err = service.ResolvePackURL(ctx, link)
			}
			if err == nil {
				source = minecraft.PackSource{Provider: v.Source, ProjectID: v.ProjectID, VersionID: v.ID, Number: v.Number}
				err = modpack.ValidateSource(source)
			}
			if err == nil {
				result, e := service.PreviewPack(ctx, v)
				preview, info, err = &result, result.Info, e
			}
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.request != id || !s.data.Open {
				return
			}
			s.cancel = nil
			s.data.Loading = false
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					s.data.Error = err.Error()
				}
				return
			}
			s.preview, s.archive, s.source = preview, archive, source
			s.data.Info = info
			s.data.Deferred = preview != nil && preview.Deferred
			s.data.OptionalAvailable = s.data.Deferred || slices.ContainsFunc(info.Files, func(file modpack.File) bool { return file.Optional })
			old := s.data.Origin.Source
			s.data.DifferentSource = old.Provider != "" && source.Provider != "" && (old.Provider != source.Provider || old.ProjectID != source.ProjectID)
		})
	})
}

func (a *app) preparePackUpdate() {
	s := &a.packUpdate
	d := &s.data
	if !d.Open || !d.HasOrigin || d.Busy || d.Loading || d.Info.Format == "" || d.Plan != nil || a.tasksFull() || a.instanceInUse(d.Key) || d.DifferentSource && !d.AllowDifferentSource {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		d.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	request := launcher.PackUpdateRequest{Path: d.Path, Optional: d.Optional, LocalFiles: slices.Clone(s.local), Source: s.source, Archive: s.archive, Preview: s.preview, AllowDifferentSource: d.AllowDifferentSource}
	ctx, token := a.beginTask(locales.UpdateModpack()+" · "+instance.Name, instance.Key(), instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate))
	if token == 0 {
		return
	}
	s.operation, s.cancel = token, a.operations[token].cancel
	commands := make(chan map[string]modpack.UpdateChoice, 1)
	s.decisions = commands
	d.Busy, d.Error, d.Status = true, "", locales.CheckingFiles()
	id := s.request
	service := a.newService()
	jobProgress := a.progressHandler(token)
	progress := func(p minecraft.Progress) {
		jobProgress(p)
		label := stageLabel(p)
		a.win.Update(func() {
			if a.ctx.Err() != nil || !a.operationCurrent(token) {
				return
			}
			if p.Download != nil {
				a.operations[token].visible = !p.Download.Finished
			}
			if s.request == id && s.data.Open {
				s.data.Status = label
			}
		})
	}
	a.workers.Go(func() {
		defer service.Close()
		prepared, err := service.PreparePackUpdate(ctx, instance, request, progress)
		backup := ""
		if err == nil {
			plan := prepared.Plan
			var counts [3]int
			var conflicts []int
			for i := range plan.Len() {
				entry := plan.Entry(i)
				counts[entry.Kind]++
				if entry.Reason != modpack.UpdateUnmodified && entry.Reason != modpack.UpdateAlreadyCurrent {
					conflicts = append(conflicts, i)
				}
			}
			a.win.Update(func() {
				if a.ctx.Err() != nil || ctx.Err() != nil || s.request != id || !s.data.Open {
					return
				}
				s.ready = prepared
				s.data.Plan, s.data.Counts, s.data.Conflicts = plan, counts, conflicts
				s.data.Info.Version = plan.Next().Pack.Version
				s.data.Choices = nil
				s.data.Busy = false
				s.data.ShowConflicts = false
				if a.operationCurrent(token) {
					a.operations[token].visible = false
				}
			})
			select {
			case <-ctx.Done():
				err = ctx.Err()
			case choices := <-commands:
				backup, err = service.ApplyPackUpdate(ctx, prepared, choices, progress, func(line string) { a.logf("%s", line) })
			}
			err = errors.Join(err, prepared.Close())
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			if s.request == id && s.data.Open {
				s.ready, s.decisions, s.cancel = nil, nil, nil
				s.data.Busy = false
				s.data.Plan, s.data.Conflicts, s.data.Choices = nil, nil, nil
				a.packUpdateSpace.close()
				if err != nil && !operationCanceled(err) {
					s.data.Error = err.Error()
					if missing, ok := errors.AsType[*modpack.MissingFilesError](err); ok {
						s.data.Missing = missing.Files
					}
				}
				if backup != "" {
					s.data.Backup = backup
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

func (a *app) applyPackUpdate() {
	s := &a.packUpdate
	if !s.data.Open || s.data.Busy || s.data.Plan == nil || s.data.Space.Blocked() || !a.operationCurrent(s.operation) {
		return
	}
	choices := maps.Clone(s.data.Choices)
	s.data.Busy, s.data.Error, s.data.Status = true, "", locales.UpdatingPackFiles()
	a.packUpdateSpace.close()
	select {
	case s.decisions <- choices:
	default:
	}
}

func (a *app) backPackUpdate() {
	s := &a.packUpdate
	if !s.data.Open || s.data.Busy || s.data.Plan == nil {
		return
	}
	s.data.Busy, s.data.Status = true, locales.Canceling()
	a.packUpdateSpace.close()
	a.cancelOperation(s.operation)
}

func (a *app) supplementPackUpdate(paths []string) {
	s := &a.packUpdate
	if !s.data.Open || s.data.Busy || len(s.data.Missing) == 0 {
		return
	}
	for _, path := range paths {
		if !slices.Contains(s.local, path) {
			s.local = append(s.local, path)
		}
	}
	s.data.LocalCount = len(s.local)
}

func (a *app) dropPackUpdate(paths []string) {
	s := &a.packUpdate
	if !s.data.Open || s.data.Busy || s.data.Loading || len(paths) == 0 || s.data.Plan != nil || s.data.Backup != "" {
		return
	}
	if len(s.data.Missing) > 0 {
		a.supplementPackUpdate(paths)
		return
	}
	if len(paths) != 1 || !isPackFile(paths[0]) {
		s.data.Error = locales.DropOneZipOrMrpackFile()
		return
	}
	s.data.Mode = 1
	a.inspectUpdatePack(paths[0], nil)
}

func (a *app) assessPackUpdateSpace() {
	s := &a.packUpdate
	if !s.data.Open || s.data.Busy || s.data.Loading || s.data.Info.Format == "" {
		return
	}
	instance, ok := a.instanceByKey(s.data.Key)
	if !ok {
		return
	}
	key := strconv.FormatUint(s.request, 10) + ":" + strconv.FormatUint(s.spaceRevision, 10)
	if s.data.Optional {
		key += ":optional"
	}
	prepared, preview := s.ready, s.preview
	info, optional := s.data.Info, s.data.Optional
	if prepared != nil {
		key += ":ready"
	}
	if a.packUpdateSpace.key == key {
		return
	}
	choices := maps.Clone(s.data.Choices)
	a.assessSpace(&a.packUpdateSpace, key, func(ctx context.Context, service *launcher.Service) (storage.Report, error) {
		if prepared != nil {
			return prepared.AssessSpace(ctx, choices)
		}
		return service.AssessPackUpdateTarget(ctx, instance, info, optional, preview)
	})
}

func (a *app) packUpdateView(c *ui.Context) {
	s := &a.packUpdate
	a.assessPackUpdateSpace()
	s.data.Space = a.packUpdateSpace.data
	a.pages.packUpdate.View(c, &s.data, page.PackUpdateActions{
		Close: a.closePackUpdate, Prepare: a.preparePackUpdate, Apply: a.applyPackUpdate, Back: a.backPackUpdate,
		SourceChanged: a.changeUpdateSource, Drop: a.dropPackUpdate,
		ChoicesChanged: func() { s.spaceRevision++ },
		Choose:         func(v packcatalog.Version) { a.inspectUpdatePack("", &v) },
		FetchURL:       func() { a.inspectUpdatePack("", nil) },
		Browse: func() {
			path := ""
			if s.data.Path != "" {
				path = filepath.Dir(s.data.Path)
			}
			a.openDirectoryPicker(pickerPackUpdate, path)
		},
		Supplement: func() { a.openDirectoryPicker(pickerPackUpdateFiles, "") },
		OpenBackup: func() {
			if s.data.Backup != "" {
				a.openDirectory(s.data.Backup)
			}
		},
	})
}
