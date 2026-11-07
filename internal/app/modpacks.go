package app

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type packDialogState struct {
	data    page.PackImportData
	request uint64
	cancel  context.CancelFunc
	local   []string
	preview *launcher.PackPreview
	source  minecraft.PackSource
	archive modpack.ArchiveIdentity
}

func (a *app) openPackImport() {
	if a.tasksFull() || a.pathBusy {
		return
	}
	a.closePackImport()
	a.dialogs.pack.data = page.PackImportData{Open: true, Step: 1, Entry: "import", Mode: "local", Directory: config.DefaultGameDirectory(), Directories: a.installDirectories()}
	a.dialogs.pack.local = nil
}

func (a *app) closePackImport() {
	a.packSpace.close()
	p := &a.dialogs.pack
	p.request++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.data = page.PackImportData{}
	p.local = nil
	p.source, p.archive = minecraft.PackSource{}, modpack.ArchiveIdentity{}
	p.preview = nil
}

func (a *app) inspectPack(path string) {
	a.inspectPackInput(path, nil, "local", "")
}

func (a *app) inspectPackInput(path string, remote *packcatalog.Version, mode, link string) {
	if a.tasksFull() || a.pathBusy {
		return
	}
	p := &a.dialogs.pack
	directory := p.data.Directory
	var project packcatalog.Project
	if mode == "online" && a.packBrowser.data.Project != nil {
		project = *a.packBrowser.data.Project
	}
	if directory == "" {
		directory = config.DefaultGameDirectory()
	}
	a.closePackImport()
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	service := a.newService()
	p.cancel = cancel
	token := p.request
	p.local = nil
	entry := "import"
	if mode == "online" {
		entry = "detail"
	}
	p.data = page.PackImportData{Open: true, Step: 2, Entry: entry, Loading: true, Path: path, Mode: mode, URL: link, Directory: directory, Directories: a.installDirectories()}
	p.data.IconURL = project.IconURL
	p.data.Icon = a.packBrowser.data.Icons[project.ID]
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		var err error
		var source minecraft.PackSource
		var preview *launcher.PackPreview
		var info modpack.Info
		var identity modpack.ArchiveIdentity
		if remote != nil || mode == "url" {
			var version packcatalog.Version
			if remote != nil {
				version = *remote
			} else {
				version, err = service.ResolvePackURL(ctx, link)
			}
			if err == nil {
				source = minecraft.PackSource{Provider: version.Source, ProjectID: version.ProjectID, VersionID: version.ID, Number: version.Number}
				err = modpack.ValidateSource(source)
			}
			if err == nil {
				var result launcher.PackPreview
				result, err = service.PreviewPack(ctx, version)
				preview, info = &result, result.Info
				if result.Deferred {
					if project.Name != "" {
						info.Name = project.Name
					}
					info.Author, info.Description = project.Author, project.Description
				}
			}
		} else {
			info, identity, err = service.InspectPack(ctx, path)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || token != p.request || !p.data.Open {
				return
			}
			p.data.Loading = false
			p.cancel = nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					p.data.Error = err.Error()
				}
				return
			}
			p.data.Path = path
			p.source, p.archive = source, identity
			p.preview = preview
			p.data.Deferred = preview != nil && preview.Deferred
			if preview != nil {
				p.data.Estimate = preview.Version.Size
			}
			p.data.Info, p.data.Name, p.data.Description = info, info.Name, info.Description
		})
	})
}

func (a *app) browsePack() {
	if !a.tasksFull() && !a.pathBusy {
		path := ""
		if a.dialogs.pack.data.Path != "" {
			path = filepath.Dir(a.dialogs.pack.data.Path)
		}
		a.openDirectoryPicker(pickerModpack, path)
	}
}

func (a *app) dropFiles(paths []string) {
	if a.worldArchive.data.Open {
		a.dropWorldArchive(paths)
		return
	}
	if a.packUpdate.data.Open {
		a.dropPackUpdate(paths)
		return
	}
	if a.tasksFull() || a.pathBusy || len(paths) == 0 {
		return
	}
	if a.dialogs.pack.data.Open {
		if len(a.dialogs.pack.data.Missing) > 0 {
			a.supplementPack(paths)
		} else if len(paths) == 1 && isPackFile(paths[0]) {
			a.inspectPack(paths[0])
		} else {
			a.dialogs.pack.data.Error = locales.DropOneZipOrMrpackFile()
		}
		return
	}
	if a.resources.data.Open && a.resources.data.MissingOpen {
		a.supplementResourcePaths(paths)
		return
	}
	if a.resources.data.Open && !a.resources.data.Online && (!a.resources.data.Embedded || a.page == pageInstances && a.pages.instances.ResourceKind() == a.resources.data.Kind) {
		a.importResourcePaths(paths)
	} else if len(paths) == 1 && isPackFile(paths[0]) {
		a.inspectPack(paths[0])
	}
}

func (a *app) supplementPack(paths []string) {
	p := &a.dialogs.pack
	for _, path := range paths {
		if !slices.Contains(p.local, path) {
			p.local = append(p.local, path)
		}
	}
	p.data.LocalCount = len(p.local)
}

func (a *app) importPack() {
	p := &a.dialogs.pack
	if a.tasksFull() || a.pathBusy || p.data.Loading || p.data.Info.Format == "" {
		return
	}
	optional := p.data.Optional
	for _, selected := range p.data.SelectedOptional {
		optional = optional || selected
	}
	request := launcher.PackImport{Path: p.data.Path, Name: p.data.Name, Description: p.data.Description, Directory: p.data.Directory, IconURL: p.data.IconURL, Optional: optional, SelectedOptional: maps.Clone(p.data.SelectedOptional), LocalFiles: slices.Clone(p.local), Source: p.source, Archive: p.archive, Preview: p.preview}
	input := request.Path
	if request.Preview != nil {
		input = request.Preview.Version.URL
	}
	ctx, service, token := a.beginOperation(locales.ImportModpack2()+request.Name, "pack:"+request.Directory+":"+input+":"+request.Name, "")
	if token == 0 {
		return
	}
	a.operations[token].installName = request.Name
	a.operations[token].failureTitle = locales.InstallationFailed()
	if request.Preview != nil {
		a.operations[token].sourceID = request.Preview.Version.OperationKey()
	}
	p.data.Busy, p.data.Open, p.data.Error, p.data.Missing = true, false, "", nil
	dialogRequest, selected := p.request, a.currentVersion()
	progress := a.progressHandler(token)
	a.logf(locales.ImportingModpack(), request.Name)
	a.workers.Go(func() {
		defer service.Close()
		instance, err := service.ImportPack(ctx, request, progress, func(line string) { a.logf("%s", line) })
		a.win.Update(func() {
			if !a.operationCurrent(token) || a.ctx.Err() != nil {
				return
			}
			if p.request == dialogRequest {
				p.data.Busy = false
			}
			if err != nil {
				if !errors.Is(err, context.Canceled) && p.request == dialogRequest {
					p.data.Error = err.Error()
					if missing, ok := errors.AsType[*modpack.MissingFilesError](err); ok {
						p.data.Open = true
						p.data.Missing = missing.Files
					}
				}
				return
			}
			if a.currentVersion() == selected {
				a.cfg.Directories.SelectedInstance = instance.Key()
			}
			a.saveConfig()
			a.scanInstalled()
			a.operations[token].resultKey = instance.Key()
			a.pages.instances.Reveal(instance.Key())
			a.logf(locales.Imported(), request.Name)
		})
		a.finishOperation(token, err)
	})
}

func (a *app) packImportView(c *ui.Context) {
	a.assessPackSpace()
	a.dialogs.pack.data.Space = a.packSpace.data
	a.dialogs.pack.data.Busy = a.tasksFull() || a.pathBusy
	page.PackImport(c, &a.dialogs.pack.data, page.PackImportActions{
		Close: a.closePackImport, Browse: a.browsePack, Import: a.importPack,
		Drop:         a.dropFiles,
		ChangeSource: a.changePackSource, Online: a.openPackBrowser,
		FetchURL: func() { a.inspectPackInput("", nil, "url", a.dialogs.pack.data.URL) },
		Supplement: func() {
			if !a.tasksFull() && !a.pathBusy {
				a.openDirectoryPicker(pickerPackFiles, "")
			}
		},
	})
	if !a.packBrowser.data.FullPage {
		a.pages.packBrowser.View(c, &a.packBrowser.data, a.packBrowserActions(false))
	}
}

func (a *app) changePackSource(mode string) {
	if a.dialogs.pack.data.Busy {
		return
	}
	if mode == "online" {
		a.openPackBrowser()
		return
	}
	p := &a.dialogs.pack
	directory := p.data.Directory
	a.closePackImport()
	if mode != "url" {
		mode = "local"
	}
	p.data = page.PackImportData{Open: true, Step: 1, Entry: "import", Mode: mode, Directory: directory, Directories: a.installDirectories()}
}

func isPackFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".zip" || ext == ".mrpack"
}
