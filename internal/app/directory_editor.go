package app

import (
	"context"
	"os"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type directoryEditor struct {
	data     page.DirectoryEditorData
	original string
	request  uint64
	cancel   context.CancelFunc
}

func (a *app) openDirectoryEditor(entry config.GameDirectory, edit bool) {
	e := &a.directoryEditor
	e.request++
	e.original = ""
	title := locales.AddGameDirectory()
	if edit {
		e.original, title = entry.Path, locales.EditGameDirectory()
	}
	e.data = page.DirectoryEditorData{Open: true, Title: title, Name: entry.Name, Path: entry.Path}
	a.pathBusy = true
}

func (a *app) closeDirectoryEditor() {
	e := &a.directoryEditor
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.request++
	e.data.Open, e.data.Saving = false, false
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) browseEditorDirectory() {
	e := &a.directoryEditor
	if !e.data.Open || e.data.Saving {
		return
	}
	path := e.data.Path
	if strings.TrimSpace(path) == "" {
		path = config.DefaultGameDirectory()
	}
	a.openDirectoryPicker(pickerGame, path)
}

func (a *app) saveDirectoryEditor() {
	e := &a.directoryEditor
	if !e.data.Open || e.data.Saving {
		return
	}
	e.request++
	token := e.request
	ctx, cancel := context.WithCancel(a.ctx)
	e.cancel = cancel
	e.data.Saving, e.data.Error = true, ""
	path, name, original := e.data.Path, e.data.Name, e.original
	a.workers.Go(func() {
		resolved, err := prepareGameDirectory(ctx, path)
		a.win.Update(func() {
			e := &a.directoryEditor
			if ctx.Err() != nil || e.request != token || !e.data.Open {
				return
			}
			cancel()
			e.cancel, e.data.Saving = nil, false
			if err == nil {
				if original == "" {
					err = a.cfg.Directories.Add(resolved, name)
				} else {
					err = a.cfg.Directories.Edit(original, resolved, name)
				}
			}
			if err != nil {
				e.data.Error = err.Error()
				return
			}
			a.saveConfig()
			a.closeDirectoryEditor()
		})
	})
}

func prepareGameDirectory(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := fsutil.DirectoryPath(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return resolveGameDirectory(path)
}

func (a *app) directoryEditorView(c *ui.Context) {
	page.DirectoryEditor(c, &a.directoryEditor.data, page.DirectoryEditorActions{
		Browse: a.browseEditorDirectory, Save: a.saveDirectoryEditor, Cancel: a.closeDirectoryEditor,
	})
}
