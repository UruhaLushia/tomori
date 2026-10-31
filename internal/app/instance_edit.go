package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type instanceEditState struct {
	data     page.InstanceEditData
	request  uint64
	original *ui.Bitmap
	cancel   context.CancelFunc
}

func (a *app) openInstanceEdit(key, mode string) {
	if a.pathBusy || a.instanceInUse(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok || !instance.CanLaunch() {
		return
	}
	a.instanceEdit.request++
	a.instanceEdit.data = page.InstanceEditData{
		Open: true, Mode: mode, Key: key, Name: instance.Name,
		Path: instance.InstanceDir, Icon: a.icons.bitmaps[key],
	}
	if mode == "rename" && instance.Format == minecraft.InstanceMinecraft {
		a.instanceEdit.data.Notice = locales.OnlyChangesTheDisplayNameInTomori()
	}
	a.pathBusy = true
	a.instanceEdit.original = nil
	a.loadInstanceIcon(instance)
	if mode == "icon" {
		token := a.instanceEdit.request
		ctx, cancel := context.WithCancel(a.ctx)
		a.instanceEdit.cancel = cancel
		a.workers.Go(func() {
			defer cancel()
			icon, _ := minecraft.LoadOriginalInstanceIcon(ctx, instance)
			var bitmap *ui.Bitmap
			if icon != nil {
				bitmap = ui.NewBitmap(icon)
			}
			a.win.Update(func() {
				if a.ctx.Err() == nil && token == a.instanceEdit.request {
					a.instanceEdit.original = bitmap
					if a.instanceEdit.data.Changed && a.instanceEdit.data.Source == "" {
						a.instanceEdit.data.Icon = bitmap
					}
				}
			})
		})
	}
}

func (a *app) closeInstanceEdit() {
	if a.instanceEdit.data.Saving {
		return
	}
	a.instanceEdit.request++
	if a.instanceEdit.cancel != nil {
		a.instanceEdit.cancel()
		a.instanceEdit.cancel = nil
	}
	a.instanceEdit.data = page.InstanceEditData{}
	a.instanceEdit.original = nil
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) saveInstanceEdit() {
	d := &a.instanceEdit.data
	if !d.Open || d.Saving || a.instanceInUse(d.Key) {
		return
	}
	instance, ok := a.instanceByKey(d.Key)
	if !ok {
		d.Error = locales.InstanceDirectoryChangedRefreshAndTryAgain()
		return
	}
	d.Saving, d.Error = true, ""
	snapshot, token := *d, a.instanceEdit.request
	directories := slices.Clone(a.cfg.Directories.Entries)
	protected := []string{config.DefaultGameDirectory()}
	for _, other := range a.installedList {
		if other.Key() != instance.Key() {
			protected = append(protected, other.InstanceDir)
		}
	}
	a.workers.Go(func() {
		var err error
		var removedRoots []string
		switch snapshot.Mode {
		case "rename":
			err = minecraft.RenameInstance(a.ctx, instance, snapshot.Name)
		case "icon":
			err = minecraft.SetInstanceIcon(a.ctx, instance, snapshot.Source)
		case "delete":
			var path string
			path, err = minecraft.InstanceTrashPath(a.ctx, instance)
			if err == nil {
				for _, directory := range directories {
					resolved := directory.Path
					if real, resolveErr := filepath.EvalSymlinks(resolved); resolveErr == nil {
						resolved = real
					}
					if rel, relErr := filepath.Rel(path, resolved); relErr == nil && rel == "." {
						removedRoots = append(removedRoots, directory.Path)
					} else {
						protected = append(protected, resolved)
					}
				}
			}
			if err == nil {
				err = protectInstanceTrash(a.ctx, path, protected)
			}
			if err == nil {
				err = mygo.Shell.TrashItem(path)
			}
		default:
			err = errors.New(locales.UnsupportedInstanceOperation())
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil || token != a.instanceEdit.request {
				return
			}
			d := &a.instanceEdit.data
			d.Saving = false
			if err != nil {
				d.Error = err.Error()
				return
			}
			scanID := a.scanID
			if snapshot.Mode == "delete" {
				delete(a.cfg.Instances, instance.Key())
				delete(a.installed, instance.Key())
				a.installedList = slices.DeleteFunc(a.installedList, func(v minecraft.Instance) bool { return v.Key() == instance.Key() })
				a.cfg.Directories.Entries = slices.DeleteFunc(a.cfg.Directories.Entries, func(v config.GameDirectory) bool {
					return slices.Contains(removedRoots, v.Path)
				})
				if a.instanceContent.data.Key == instance.Key() {
					if a.instanceContent.cancel != nil {
						a.instanceContent.cancel()
					}
					a.instanceContent.request++
					a.releaseInstanceMedia()
					a.instanceContent.data = page.InstanceContentData{}
					a.closeResources()
				}
				a.cfg.Directories.SelectedInstance = a.currentVersion()
				a.saveConfig()
			}
			a.closeInstanceEdit()
			if a.scanID == scanID {
				a.scanInstalled()
			}
		})
	})
}

func protectInstanceTrash(ctx context.Context, target string, paths []string) error {
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			path = resolved
		}
		rel, err := filepath.Rel(target, path)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return errors.New(locales.ThisDirectoryContainsOtherInstancesOrTheBuiltInGameDirectoryAndCannotBeDeleted())
		}
	}
	return ctx.Err()
}

func (a *app) instanceEditView(c *ui.Context) {
	page.InstanceEdit(c, &a.instanceEdit.data, page.InstanceEditActions{
		Save: a.saveInstanceEdit, Close: a.closeInstanceEdit,
		Browse: func() { a.openDirectoryPicker(pickerInstanceIcon, a.instanceEdit.data.Path) },
		Reset: func() {
			a.instanceEdit.data.Source, a.instanceEdit.data.Error = "", ""
			a.instanceEdit.data.Icon, a.instanceEdit.data.Changed = a.instanceEdit.original, true
		},
	})
}

func (a *app) confirmInstanceIcon(ctx context.Context, token uint64, path string) {
	p := &a.directoryPicker
	p.data.Saving, p.data.Error = true, ""
	key, win := p.instanceKey, p.win
	a.workers.Go(func() {
		bitmap, err := readIconPreview(ctx, path)
		win.Update(func() {
			if !a.pickerCurrent(ctx, token) {
				return
			}
			p := &a.directoryPicker
			p.data.Saving = false
			if err != nil {
				p.data.Error = err.Error()
				return
			}
			d := &a.instanceEdit.data
			if !d.Open || d.Key != key || d.Mode != "icon" {
				a.closeDirectoryPicker()
				return
			}
			d.Source, d.Icon, d.Changed, d.Error = path, bitmap, true, ""
			a.closeDirectoryPicker()
			a.win.Invalidate()
		})
	})
}

func readIconPreview(ctx context.Context, path string) (*ui.Bitmap, error) {
	icon, err := minecraft.ReadInstanceIcon(ctx, path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ui.NewBitmap(icon), nil
}
