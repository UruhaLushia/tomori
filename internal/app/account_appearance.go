package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/locales"
)

type appearanceState struct {
	data                               page.AppearanceData
	id                                 string
	request, skinID, capeID, operation uint64
	ctx                                context.Context
	cancel, skinCancel, capeCancel     context.CancelFunc
	profile                            account.Profile
	raw                                []byte
	local                              bool
}

func (a *app) appearanceAccount() (account.Account, bool) {
	for _, identity := range a.cfg.Accounts.Entries {
		if identity.ID == a.appearance.id && identity.Kind == account.Microsoft {
			return identity, true
		}
	}
	return account.Account{}, false
}

func (a *app) openAppearance(id string) {
	if a.pathBusy || a.authBusy || a.preparingGames() || a.tasksFull() {
		return
	}
	s := &a.appearance
	s.id = id
	identity, ok := a.appearanceAccount()
	if !ok {
		s.id = ""
		return
	}
	s.data = page.AppearanceData{Open: true, Name: identity.Name, Variant: "classic", Fallback: a.avatars[id].bitmap}
	a.pathBusy = true
	a.refreshAppearance()
}

func (a *app) closeAppearance() {
	s := &a.appearance
	if s.cancel != nil {
		s.cancel()
	}
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.skinID++
	s.capeID++
	s.data, s.profile, s.raw = page.AppearanceData{}, account.Profile{}, nil
	s.ctx, s.cancel, s.skinCancel, s.capeCancel, s.id, s.operation = nil, nil, nil, nil, "", 0
	if a.directoryPicker.data.Open && (a.directoryPicker.kind == pickerSkin || a.directoryPicker.kind == pickerSkinExport) {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) refreshAppearance() {
	s := &a.appearance
	identity, ok := a.appearanceAccount()
	if !s.data.Open || s.data.Busy || !ok {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	ctx, cancel := context.WithCancel(a.ctx)
	s.ctx, s.cancel = ctx, cancel
	token := s.request
	s.raw, s.local, s.profile = nil, false, account.Profile{}
	s.data.Loading, s.data.Ready, s.data.Modified, s.data.CanUpload, s.data.Previewing = true, false, false, false, false
	s.data.Error, s.data.FileName, s.data.CapeID, s.data.ActiveCape = "", "", "", ""
	s.data.Skin, s.data.Cape, s.data.Capes = [2]*ui.Bitmap{}, nil, nil
	budget, proxy, path := a.downloadBudget, a.cfg.Download.Proxy, filepath.Join(a.store.Dir(), fmt.Sprintf("account-%x", sha256.Sum256([]byte(identity.ID))))
	a.workers.Go(func() {
		updated, err := a.ensureAccount(ctx, budget, path, proxy, identity)
		client := account.NewClient(proxy, budget)
		defer client.Close()
		var profile account.Profile
		if err == nil {
			profile, err = client.Profile(ctx, updated)
		}
		var raw []byte
		var views [2]*ui.Bitmap
		var cape *ui.Bitmap
		ready := err == nil
		variant := "classic"
		fileName := locales.DefaultSkin()
		if ready {
			if skin, ok := profile.ActiveSkin(); ok {
				variant = strings.ToLower(skin.Variant)
				if variant != "slim" {
					variant = "classic"
				}
				raw, err = client.Texture(ctx, skin.URL)
				if err == nil {
					views, err = appearancePreview(raw, variant)
				}
				fileName = locales.CurrentSkin()
			}
			for _, entry := range profile.Capes {
				if entry.State == "ACTIVE" {
					texture, e := client.Texture(ctx, entry.URL)
					if e == nil {
						img, imageErr := account.CapePreview(texture)
						e = imageErr
						if e == nil {
							cape = ui.NewBitmap(img)
						}
					}
					if err == nil {
						err = e
					}
					break
				}
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != token || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.data.Loading, s.data.Ready, s.data.Variant = false, ready, variant
			s.data.Skin, s.data.Cape, s.data.FileName = views, cape, fileName
			s.raw, s.profile = raw, profile
			if info, e := png.DecodeConfig(bytes.NewReader(raw)); e == nil {
				s.data.CanUpload = info.Width == 64
			}
			if err != nil {
				s.data.Error = err.Error()
			}
			if !ready {
				return
			}
			s.data.Capes, s.data.CapeID, s.data.ActiveCape = profile.Capes, profile.ActiveCape(), profile.ActiveCape()
			changed := a.applyAccountUpdates()
			if current, ok := a.appearanceAccount(); ok {
				skin, _ := profile.ActiveSkin()
				if current.SkinURL != skin.URL || current.Name != profile.Name {
					current.SkinURL, current.Name = skin.URL, profile.Name
					a.putAccount(current, false)
					changed = true
				}
				s.data.Name, s.data.Fallback = current.Name, a.avatars[current.ID].bitmap
			}
			if changed {
				a.saveConfig()
			}
		})
	})
}

func appearancePreview(raw []byte, variant string) ([2]*ui.Bitmap, error) {
	var views [2]*ui.Bitmap
	images, err := account.SkinPreview(raw, variant)
	if err != nil {
		return views, err
	}
	for i, img := range images {
		views[i] = ui.NewBitmap(img)
	}
	return views, nil
}

func (a *app) loadAppearanceSkin(paths []string) {
	s := &a.appearance
	if !s.data.Open || s.data.Loading || s.data.Busy {
		return
	}
	if len(paths) != 1 {
		s.data.Error = locales.SelectOnePngSkin()
		return
	}
	a.renderAppearanceSkin(paths[0])
}

func (a *app) renderAppearanceSkin(path string) {
	s := &a.appearance
	if !s.data.Open || s.data.Loading || s.data.Busy {
		return
	}
	if s.skinCancel != nil {
		s.skinCancel()
	}
	s.skinID++
	ctx, cancel := context.WithCancel(s.ctx)
	s.skinCancel = cancel
	token, viewID, variant, raw, budget := s.skinID, s.request, s.data.Variant, s.raw, a.downloadBudget
	s.data.Busy, s.data.Error = true, ""
	a.workers.Go(func() {
		defer cancel()
		var err error
		if path != "" {
			var release func()
			release, err = budget.FilePermit(ctx)
			if err == nil {
				raw, err = account.ReadSkinFile(ctx, path)
				release()
			}
		}
		var views [2]*ui.Bitmap
		if err == nil {
			views, err = appearancePreview(raw, variant)
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != viewID || s.skinID != token || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.data.Busy = false
			if err != nil {
				s.data.Error = err.Error()
				return
			}
			s.raw, s.data.Skin = raw, views
			if path != "" {
				s.local = true
				s.data.FileName = filepath.Base(path)
			}
			current, _ := s.profile.ActiveSkin()
			s.data.Modified = s.local || !strings.EqualFold(current.Variant, variant)
			info, _ := png.DecodeConfig(bytes.NewReader(raw))
			s.data.CanUpload = info.Width == 64
		})
	})
}

func (a *app) previewAppearanceCape() {
	s := &a.appearance
	if !s.data.Open || !s.data.Ready || s.data.Busy {
		return
	}
	if s.capeCancel != nil {
		s.capeCancel()
	}
	s.capeID++
	s.data.Cape, s.data.Error, s.data.Previewing = nil, "", false
	if s.data.CapeID == "" {
		return
	}
	var address string
	for _, cape := range s.profile.Capes {
		if cape.ID == s.data.CapeID {
			address = cape.URL
			break
		}
	}
	if address == "" {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.capeCancel = cancel
	viewID, token, proxy, budget := s.request, s.capeID, a.cfg.Download.Proxy, a.downloadBudget
	s.data.Previewing = true
	a.workers.Go(func() {
		defer cancel()
		client := account.NewClient(proxy, budget)
		defer client.Close()
		raw, err := client.Texture(ctx, address)
		var bitmap *ui.Bitmap
		if err == nil {
			img, e := account.CapePreview(raw)
			err = e
			if err == nil {
				bitmap = ui.NewBitmap(img)
			}
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != viewID || s.capeID != token || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.data.Previewing, s.data.Cape = false, bitmap
			if err != nil {
				s.data.Error = err.Error()
			}
		})
	})
}

func (a *app) changeAppearance(mode string) {
	s := &a.appearance
	identity, ok := a.appearanceAccount()
	if !s.data.Open || s.data.Loading || s.data.Busy || !ok || a.tasksFull() {
		return
	}
	if mode == "skin" && (!s.data.Modified || !s.data.CanUpload) || mode == "cape" && (!s.data.Ready || s.data.CapeID == s.data.ActiveCape) || mode != "skin" && mode != "cape" && mode != "reset" {
		return
	}
	ctx, operation := a.beginTask(locales.UpdateAppearance(), "appearance:"+identity.ID, "")
	if operation == 0 {
		return
	}
	s.data.Busy, s.data.Error, s.operation = true, "", operation
	viewID, raw, variant, cape := s.request, s.raw, s.data.Variant, s.data.CapeID
	budget, proxy, path := a.downloadBudget, a.cfg.Download.Proxy, filepath.Join(a.store.Dir(), fmt.Sprintf("account-%x", sha256.Sum256([]byte(identity.ID))))
	a.workers.Go(func() {
		updated, err := a.ensureAccount(ctx, budget, path, proxy, identity)
		client := account.NewClient(proxy, budget)
		defer client.Close()
		if err == nil {
			switch mode {
			case "skin":
				err = client.UploadSkin(ctx, updated, raw, variant)
			case "cape":
				err = client.SetCape(ctx, updated, cape)
			case "reset":
				err = client.ResetSkin(ctx, updated)
			}
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != viewID || !s.data.Open || s.operation != operation || a.ctx.Err() != nil {
				return
			}
			s.data.Busy, s.operation = false, 0
			if err != nil {
				if !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				return
			}
			s.data.ConfirmReset = false
			a.refreshAppearance()
		})
		a.finishOperation(operation, err)
	})
}

func (a *app) saveAppearance(directory string, viewID uint64) {
	s := &a.appearance
	if !s.data.Open || s.data.Busy || s.request != viewID || len(s.raw) == 0 || a.tasksFull() {
		return
	}
	ctx, operation := a.beginTask(locales.SaveSkin(), "appearance:"+s.id, "")
	if operation == 0 {
		return
	}
	raw, name, budget := s.raw, s.data.Name+"-skin.png", a.downloadBudget
	s.data.Busy, s.data.Error, s.operation = true, "", operation
	a.workers.Go(func() {
		release, err := budget.FilePermit(ctx)
		if err == nil {
			_, err = account.ExportSkin(ctx, directory, name, raw)
			release()
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != viewID || !s.data.Open || s.operation != operation || a.ctx.Err() != nil {
				return
			}
			s.data.Busy, s.operation = false, 0
			if err != nil && !operationCanceled(err) {
				s.data.Error = err.Error()
			}
		})
		a.finishOperation(operation, err)
	})
}

func (a *app) appearanceView(c *ui.Context) {
	if a.appearance.data.Open {
		a.appearance.data.Fallback = a.avatars[a.appearance.id].bitmap
	}
	page.AccountAppearance(c, &a.appearance.data, page.AppearanceActions{Close: a.closeAppearance, Refresh: a.refreshAppearance, Browse: func() { a.openDirectoryPicker(pickerSkin, "") }, Model: func() {
		if len(a.appearance.raw) > 0 {
			a.renderAppearanceSkin("")
		}
	}, PreviewCape: a.previewAppearanceCape, Save: func() { a.openDirectoryPicker(pickerSkinExport, "") }, Upload: func() { a.changeAppearance("skin") }, Reset: func() { a.changeAppearance("reset") }, ApplyCape: func() { a.changeAppearance("cape") }, Drop: a.loadAppearanceSkin})
}
