package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type directoryPickerKind uint8

const (
	pickerGame directoryPickerKind = iota
	pickerCache
	pickerResource
	pickerModpack
	pickerPackFiles
	pickerJava
	pickerJavaAdd
	pickerInstanceJava
	pickerInstanceIcon
	pickerResourceFiles
	pickerPackExport
	pickerPackUpdate
	pickerPackUpdateFiles
	pickerPackAssociate
	pickerWorldFile
	pickerWorldDirectory
	pickerResourceDirectory
	pickerLogReport
	pickerScreenshotExport
	pickerSkin
	pickerSkinExport
	pickerShortcut
)

type directoryPicker struct {
	data               page.DirectoryPickerData
	kind               directoryPickerKind
	request            uint64
	cancel             context.CancelFunc
	history            []string
	position           int
	win                *mygo.Window
	closing            bool
	instanceKey        string
	resourceRequest    uint64
	exportRequest      uint64
	updateRequest      uint64
	associationRequest uint64
	worldRequest       uint64
	resourceView       uint64
	logRequest         uint64
	screenshotRequest  uint64
	appearanceRequest  uint64
}

func (a *app) openDirectoryPicker(kind directoryPickerKind, path string) {
	p := &a.directoryPicker
	if p.win != nil {
		p.win.Focus()
		return
	}
	if strings.TrimSpace(path) == "" {
		path = p.data.Current
		if path == "" {
			path, _ = mygo.App.Path(mygo.PathDownloads)
		}
		if path == "" {
			path, _ = os.UserHomeDir()
		}
	}
	p.request++
	p.kind = kind
	p.instanceKey = ""
	p.resourceRequest = a.resources.missingID
	p.exportRequest = a.packExport.request
	p.updateRequest = a.packUpdate.request
	p.associationRequest = a.packAssociate.request
	p.worldRequest = a.worldArchive.request
	p.resourceView = a.resources.data.ID
	p.logRequest = a.logDiagnostic.request
	p.screenshotRequest = a.screenshotEdit.request
	p.appearanceRequest = a.appearance.request
	if kind == pickerInstanceJava {
		p.instanceKey = a.instanceSettings.Key
	}
	if kind == pickerInstanceIcon {
		p.instanceKey = a.instanceEdit.data.Key
	}
	p.history, p.position = nil, -1
	title := locales.ChooseGameDirectory()
	switch kind {
	case pickerWorldFile:
		title = locales.ChooseWorldArchive()
	case pickerWorldDirectory:
		title = locales.ChooseWorldFolder()
	case pickerLogReport, pickerScreenshotExport, pickerSkinExport:
		title = locales.ChooseExportFolder()
	case pickerShortcut:
		title = locales.ShortcutLocation()
	case pickerSkin:
		title = locales.ChooseSkin()
	}
	if kind == pickerCache {
		title = locales.ChooseCacheDirectory()
	}
	if kind == pickerPackExport {
		title = locales.ChooseExportFolder()
	}
	if kind == pickerResource || kind == pickerResourceDirectory {
		title = locales.Import() + a.resources.data.Kind.Label()
	}
	if kind == pickerResourceFiles {
		title = locales.AddMissingResourceFiles()
	}
	if kind == pickerModpack || kind == pickerPackUpdate || kind == pickerPackAssociate {
		title = locales.ChooseModpack()
	}
	if kind == pickerPackFiles || kind == pickerPackUpdateFiles {
		title = locales.AddMissingModpackFiles()
	}
	if kind == pickerJava || kind == pickerJavaAdd || kind == pickerInstanceJava {
		title = locales.ChooseJava()
	}
	if kind == pickerInstanceIcon {
		title = locales.ChooseImage()
	}
	p.data = page.DirectoryPickerData{ID: p.request, Open: true, Title: title, Path: path}
	if kind == pickerSkin {
		p.data.Files, p.data.SingleFile = true, true
		p.data.Path = ""
		p.data.Extensions = []string{".png"}
		p.data.ConfirmLabel = locales.Select()
	}
	if kind == pickerWorldFile {
		p.data.Files, p.data.SingleFile = true, true
		p.data.Path = ""
		p.data.Extensions = []string{".zip"}
		p.data.ConfirmLabel = locales.Select()
	}
	if kind == pickerResource {
		p.data.Files = true
		p.data.Path = ""
		p.data.Extensions = []string{".zip", ".zip.disabled"}
		if a.resources.data.Kind == resource.DataPack {
			p.data.Extensions = []string{".zip"}
		}
		if a.resources.data.Kind == resource.Mod {
			p.data.Extensions = append(p.data.Extensions, ".jar", ".jar.disabled", ".litemod", ".litemod.disabled")
		}
	}
	if kind == pickerResourceFiles {
		p.data.Files = true
		p.data.Path = ""
		p.data.Extensions = []string{".zip", ".jar", ".litemod", ".zip.disabled", ".jar.disabled", ".litemod.disabled"}
		p.data.ConfirmLabel = locales.Select()
	}
	if kind == pickerModpack || kind == pickerPackFiles || kind == pickerPackUpdate || kind == pickerPackUpdateFiles || kind == pickerPackAssociate {
		p.data.Files = true
		p.data.Path = ""
		p.data.Extensions = []string{".zip", ".mrpack", ".jar", ".litemod"}
		if kind == pickerModpack || kind == pickerPackUpdate || kind == pickerPackAssociate {
			p.data.Extensions = []string{".zip", ".mrpack"}
			p.data.SingleFile = true
		}
	}
	if kind == pickerJava || kind == pickerJavaAdd || kind == pickerInstanceJava {
		p.data.Files, p.data.SingleFile = true, true
		p.data.Path = ""
		p.data.FileNames = java.ExecutableNames()
		p.data.Extensions = []string{""}
		p.data.ConfirmLabel = locales.Select()
	}
	a.pathBusy = true
	if kind == pickerInstanceIcon {
		p.data.Files, p.data.SingleFile = true, true
		p.data.Path = ""
		p.data.Extensions = []string{".png", ".jpg", ".jpeg", ".webp", ".gif"}
		p.data.ConfirmLabel = locales.Select()
	}
	if kind == pickerCache {
		a.cacheError, a.cacheStatus = "", ""
	}
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title: title, Width: 760, Height: 560, MinWidth: 640, MinHeight: 480,
		UseContentSize: true, TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 40,
		Parent: a.win, Modal: true, SkipTaskbar: true,
		HideWindowButtons: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		BackgroundColor: kit.HexColor(kit.Surface), Content: ui.View(a.directoryPickerView),
	})
	p.win.OnClose(func(event *mygo.CloseEvent) {
		event.PreventDefault()
		a.closeDirectoryPicker()
	})
	p.win.OnClosed(func() {
		p.win, p.closing = nil, false
		a.resetDirectoryPicker()
		a.win.Invalidate()
	})
	a.readPickerDirectory(path, true, -1)
}

func (a *app) pickerContext() (context.Context, uint64) {
	p := &a.directoryPicker
	if p.cancel != nil {
		p.cancel()
	}
	p.request++
	ctx, cancel := context.WithCancel(a.ctx)
	p.cancel = cancel
	return ctx, p.request
}

func (a *app) pickerCurrent(ctx context.Context, token uint64) bool {
	return ctx.Err() == nil && a.directoryPicker.request == token && a.directoryPicker.data.Open
}

func (a *app) browseDirectory(path string) {
	if !a.directoryPicker.data.Open || a.directoryPicker.data.Saving {
		return
	}
	a.readPickerDirectory(pickerPath(path, a.directoryPicker.data.Current), false, -1)
}

func pickerPath(path, current string) string {
	if path != "" && !filepath.IsAbs(path) && path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return filepath.Join(current, path)
	}
	return path
}

func (a *app) movePickerHistory(delta int) {
	p := &a.directoryPicker
	position := p.position + delta
	if !p.data.Open || p.data.Saving || position < 0 || position >= len(p.history) {
		return
	}
	a.readPickerDirectory(p.history[position], false, position)
}

func (a *app) readPickerDirectory(path string, initial bool, position int) {
	p := &a.directoryPicker
	ctx, token := a.pickerContext()
	p.data.Loading, p.data.Error = true, ""
	locations := p.data.Locations
	files, extensions := p.data.Files, append([]string(nil), p.data.Extensions...)
	names := slices.Clone(p.data.FileNames)
	win := p.win
	a.workers.Go(func() {
		current, entries, err := fsutil.ReadEntries(ctx, path, initial, files, extensions)
		if len(names) > 0 {
			entries = slices.DeleteFunc(entries, func(entry fsutil.DirectoryEntry) bool {
				return !entry.Directory && !slices.Contains(names, strings.ToLower(entry.Name))
			})
		}
		if locations == nil {
			locations = pickerLocations(ctx)
		}
		win.Update(func() {
			if !a.pickerCurrent(ctx, token) {
				return
			}
			data := &a.directoryPicker.data
			data.Loading, data.Locations = false, locations
			if err != nil {
				data.Error = err.Error()
				return
			}
			data.Current, data.Directories = current, entries
			p := &a.directoryPicker
			if position >= 0 {
				p.position = position
			} else if p.position < 0 || p.history[p.position] != current {
				p.history = append(p.history[:p.position+1], current)
				p.position = len(p.history) - 1
			}
			data.CanBack, data.CanForward = p.position > 0, p.position+1 < len(p.history)
			if data.Files {
				data.Path = ""
				data.SelectedFiles = nil
			} else if !initial {
				data.Path = current
			}
		})
	})
}

func (a *app) closeDirectoryPicker() {
	p := &a.directoryPicker
	if p.closing {
		return
	}
	a.resetDirectoryPicker()
	if win := p.win; win != nil {
		p.closing = true
		// Input actions can run inside GtkGLArea's render callback. Let GTK
		// finish the callback before destroying the area and its GL context.
		win.Update(func() {
			if p.win == win {
				win.Destroy()
			}
		})
	}
}

func (a *app) resetDirectoryPicker() {
	p := &a.directoryPicker
	wasOpen := p.data.Open
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.request++
	p.data.Open, p.data.Loading, p.data.Saving = false, false, false
	p.data.Directories, p.data.SelectedFiles = nil, nil
	p.history = nil
	a.pages.directoryPicker = page.DirectoryPicker{}
	if wasOpen {
		a.pathBusy = a.pathDialogsOpen()
	}
}

func (a *app) createPickerDirectory(name string) {
	p := &a.directoryPicker
	if !p.data.Open || p.data.Saving || p.data.Loading || p.data.Current == "" {
		return
	}
	if err := fsutil.ValidateID(name); err != nil {
		p.data.Error = locales.InvalidFolderName()
		return
	}
	path := filepath.Join(p.data.Current, name)
	ctx, token := a.pickerContext()
	p.data.Saving, p.data.Error = true, ""
	win := p.win
	a.workers.Go(func() {
		err := ctx.Err()
		if err == nil {
			err = os.Mkdir(path, 0o755)
		}
		win.Update(func() {
			if !a.pickerCurrent(ctx, token) {
				return
			}
			a.directoryPicker.data.Saving = false
			if err != nil {
				a.directoryPicker.data.Error = err.Error()
				return
			}
			a.browseDirectory(path)
		})
	})
}

func (a *app) confirmDirectoryPicker() {
	p := &a.directoryPicker
	if !p.data.Open || p.data.Saving || p.data.Loading {
		return
	}
	ctx, token := a.pickerContext()
	path, kind := p.data.Path, p.kind
	exportRequest := p.exportRequest
	updateRequest := p.updateRequest
	worldRequest := p.worldRequest
	logRequest := p.logRequest
	screenshotRequest := p.screenshotRequest
	appearanceRequest := p.appearanceRequest
	associationRequest := p.associationRequest
	instanceKey := p.instanceKey
	if p.data.Files {
		paths := append([]string(nil), p.data.SelectedFiles...)
		if len(paths) == 0 && strings.TrimSpace(path) != "" && path != p.data.Current {
			paths = []string{pickerPath(path, p.data.Current)}
		}
		if len(paths) == 0 {
			p.data.Error = locales.SelectFiles()
			return
		}
		if (kind == pickerModpack || kind == pickerPackUpdate || kind == pickerPackAssociate) && len(paths) != 1 {
			p.data.Error = locales.SelectOneModpack()
			return
		}
		if kind == pickerJava || kind == pickerJavaAdd || kind == pickerInstanceJava {
			if len(paths) != 1 {
				p.data.Error = locales.SelectOneJavaExecutable()
				return
			}
			a.confirmJavaFile(ctx, token, pickerPath(paths[0], p.data.Current))
			return
		}
		if kind == pickerInstanceIcon {
			if len(paths) != 1 {
				p.data.Error = locales.ChooseOneImage()
				return
			}
			a.confirmInstanceIcon(ctx, token, pickerPath(paths[0], p.data.Current))
			return
		}
		if kind == pickerWorldFile && len(paths) != 1 {
			p.data.Error = locales.ChooseOneWorldFolderOrZipFile()
			return
		}
		if kind == pickerSkin && len(paths) != 1 {
			p.data.Error = locales.SelectOnePngSkin()
			return
		}
		resourceRequest := p.resourceRequest
		resourceView := p.resourceView
		a.closeDirectoryPicker()
		switch kind {
		case pickerSkin:
			if a.appearance.request == appearanceRequest {
				a.loadAppearanceSkin(paths)
			}
		case pickerWorldFile:
			a.chooseWorldArchive(paths[0], worldRequest)
		case pickerModpack:
			a.inspectPack(paths[0])
		case pickerPackFiles:
			a.supplementPack(paths)
		case pickerPackUpdate:
			if a.packUpdate.request == updateRequest {
				a.inspectUpdatePack(paths[0], nil)
			}
		case pickerPackAssociate:
			if a.packAssociate.request == associationRequest {
				a.inspectAssociation(paths[0])
			}
		case pickerPackUpdateFiles:
			if a.packUpdate.request == updateRequest {
				a.supplementPackUpdate(paths)
			}
		case pickerResourceFiles:
			if a.resources.missingID == resourceRequest {
				a.supplementResourcePaths(paths)
			}
		case pickerResource:
			if a.resources.data.Open && a.resources.data.ID == resourceView {
				a.importResourcePaths(paths)
			}
		}
		return
	}
	path = pickerPath(path, p.data.Current)
	resourceView := p.resourceView
	p.data.Saving, p.data.Error = true, ""
	win := p.win
	a.workers.Go(func() {
		resolved, err := selectedDirectory(ctx, path)
		if err == nil && kind == pickerCache {
			err = checkCacheDirectory(resolved)
		}
		win.Update(func() {
			if !a.pickerCurrent(ctx, token) {
				return
			}
			p := &a.directoryPicker
			p.data.Saving = false
			if err == nil {
				if kind == pickerCache {
					a.cfg.Download.CacheDirectory = resolved
				} else if kind == pickerWorldDirectory {
					a.chooseWorldArchive(resolved, worldRequest)
				} else if kind == pickerPackExport {
					a.chooseExportDirectory(resolved, exportRequest)
				} else if kind == pickerLogReport {
					a.chooseLogReportDirectory(resolved, logRequest)
				} else if kind == pickerScreenshotExport {
					a.chooseScreenshotDirectory(resolved, screenshotRequest)
				} else if kind == pickerSkinExport {
					a.saveAppearance(resolved, appearanceRequest)
				} else if kind != pickerShortcut && a.directoryEditor.data.Open {
					a.directoryEditor.data.Path = resolved
					a.directoryEditor.data.Error = ""
				}
			}
			if err != nil {
				p.data.Error = err.Error()
				return
			}
			if kind == pickerCache {
				a.saveConfig()
			}
			a.win.Invalidate()
			a.closeDirectoryPicker()
			if kind == pickerShortcut {
				a.createInstanceShortcut(instanceKey, resolved)
			}
			if kind == pickerResourceDirectory && a.resources.data.Open && a.resources.data.ID == resourceView {
				a.importResourcePaths([]string{resolved})
			}
			if kind == pickerCache {
				a.prepareCache()
			}
		})
	})
}

func selectedDirectory(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := fsutil.DirectoryPath(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf(locales.NotADirectory(), path)
	}
	return path, nil
}

func checkCacheDirectory(path string) error {
	probe, err := os.CreateTemp(path, ".tomori-probe-*")
	if err != nil {
		return err
	}
	err = probe.Close()
	_ = os.Remove(probe.Name())
	return err
}

func (a *app) directoryPickerView(c *ui.Context) {
	a.applyAppearance(c.Theme().Dark)
	c.SetTheme(kit.Theme())
	a.pages.directoryPicker.View(c, &a.directoryPicker.data, page.DirectoryPickerActions{
		Navigate: a.browseDirectory, Create: a.createPickerDirectory,
		Confirm: a.confirmDirectoryPicker, Cancel: a.closeDirectoryPicker,
		Back: func() { a.movePickerHistory(-1) }, Forward: func() { a.movePickerHistory(1) },
	})
}
