package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

type appearanceState struct {
	data                               page.AppearanceData
	id                                 string
	request, skinID, capeID, operation uint64
	ctx                                context.Context
	cancel, skinCancel, capeCancel     context.CancelFunc
	profile                            account.Profile
	raw, remoteRaw                     []byte
	capeRaw                            []byte
	yaw, pitch                         float32
	skinPath                           string
	library                            []appearanceSkin
	local                              bool
	remoteVariant                      string
}

type appearanceSkin struct {
	id, path, name string
	preview        image.Image
}

func (a *app) appearanceAccount() (account.Account, bool) {
	for _, identity := range a.cfg.Accounts.Entries {
		if identity.ID == a.appearance.id && identity.Kind == account.Microsoft {
			return identity, true
		}
	}
	return account.Account{}, false
}

func appearanceSkinDir() string { return filepath.Join(config.DataDir(), "skins") }

func (a *app) scanAppearanceSkins(ctx context.Context) []appearanceSkin {
	entries, err := os.ReadDir(appearanceSkinDir())
	if err != nil {
		return nil
	}
	result := make([]appearanceSkin, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".png" {
			continue
		}
		path := filepath.Join(appearanceSkinDir(), entry.Name())
		raw, err := account.ReadSkinFile(ctx, path)
		if err != nil {
			continue
		}
		variant, err := account.DetectSkinVariant(raw)
		if err != nil {
			continue
		}
		views, err := account.SkinPreview(raw, variant)
		if err != nil {
			continue
		}
		result = append(result, appearanceSkin{id: path, path: path, name: strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())), preview: views[0]})
	}
	return result
}

func (a *app) appearanceSkinItems() []page.SkinItem {
	items := make([]page.SkinItem, 0, len(a.appearance.library)+1)
	if len(a.appearance.remoteRaw) > 0 {
		views, _ := account.SkinPreview(a.appearance.remoteRaw, a.appearance.remoteVariant)
		if views[0] != nil {
			items = append(items, page.SkinItem{ID: "current", Name: locales.CurrentSkin(), Preview: ui.NewBitmap(views[0]), Selected: !a.appearance.local})
		}
	}
	for _, skin := range a.appearance.library {
		items = append(items, page.SkinItem{ID: skin.id, Name: skin.name, Preview: ui.NewBitmap(skin.preview), Selected: a.appearance.local && a.appearance.skinPath == skin.path})
	}
	return items
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
	s.data = page.AppearanceData{Open: true, Name: identity.Name, Variant: "classic", ModelAuto: true, Fallback: a.avatars[id].bitmap}
	s.library, s.skinPath, s.remoteRaw, s.remoteVariant = nil, "", nil, "classic"
	s.yaw, s.pitch = 0, 0
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
	s.data, s.profile, s.raw, s.remoteRaw, s.capeRaw = page.AppearanceData{}, account.Profile{}, nil, nil, nil
	s.library, s.skinPath, s.remoteVariant = nil, "", ""
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
	s.raw, s.remoteRaw, s.capeRaw, s.local, s.profile = nil, nil, nil, false, account.Profile{}
	s.skinPath, s.remoteVariant = "", "classic"
	s.data.Loading, s.data.Ready, s.data.Modified, s.data.CanUpload, s.data.Previewing, s.data.ModelAuto = true, false, false, false, false, true
	s.data.Error, s.data.FileName, s.data.CapeID, s.data.ActiveCape = "", "", "", ""
	s.data.Preview, s.data.Capes = nil, nil
	budget, proxy, path := a.downloadBudget, a.cfg.Download.Proxy, filepath.Join(a.store.Dir(), fmt.Sprintf("account-%x", sha256.Sum256([]byte(identity.ID))))
	a.workers.Go(func() {
		library := a.scanAppearanceSkins(ctx)
		updated, err := a.ensureAccount(ctx, budget, path, proxy, identity)
		client := account.NewClient(proxy, budget)
		defer client.Close()
		var profile account.Profile
		if err == nil {
			profile, err = client.Profile(ctx, updated)
		}
		var raw []byte
		var capeRaw []byte
		ready := err == nil
		variant := "classic"
		fileName := locales.DefaultSkin()
		if ready {
			var skinURL, capeURL string
			if skin, ok := profile.ActiveSkin(); ok {
				variant = strings.ToLower(skin.Variant)
				if variant != "slim" {
					variant = "classic"
				}
				skinURL = skin.URL
				fileName = locales.CurrentSkin()
			}
			for _, entry := range profile.Capes {
				if entry.State == "ACTIVE" {
					capeURL = entry.URL
					break
				}
			}
			var wg sync.WaitGroup
			var skinErr, capeErr error
			if skinURL != "" {
				wg.Go(func() {
					raw, skinErr = client.TextureCached(ctx, a.cfg.CacheDirectory(), skinURL)
				})
			}
			if capeURL != "" {
				wg.Go(func() {
					var texture []byte
					texture, capeErr = client.TextureCached(ctx, a.cfg.CacheDirectory(), capeURL)
					if capeErr == nil {
						_, capeErr = account.CapePreview(texture)
						if capeErr == nil {
							capeRaw = texture
						}
					}
				})
			}
			wg.Wait()
			if skinErr != nil {
				err = skinErr
			}
			if err == nil && capeErr != nil {
				err = capeErr
			}
			if err == nil && len(raw) > 0 {
				if detected, detectErr := account.DetectSkinVariant(raw); detectErr == nil {
					variant = detected
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
			s.data.Loading, s.data.Ready, s.data.Variant, s.data.ModelAuto = false, ready, variant, true
			s.data.FileName = fileName
			s.library, s.raw, s.remoteRaw, s.capeRaw, s.profile = library, raw, raw, capeRaw, profile
			s.remoteVariant = variant
			s.data.Skins = a.appearanceSkinItems()
			if len(raw) > 0 {
				if rendered, renderErr := account.RenderSkin(raw, capeRaw, variant, 360, 420, s.yaw, s.pitch); renderErr == nil {
					s.data.Preview = ui.NewBitmap(rendered)
				}
			}
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

func (a *app) selectAppearanceSkin(id string) {
	s := &a.appearance
	if !s.data.Open || s.data.Loading || s.data.Busy {
		return
	}
	if id == "current" {
		a.renderAppearanceSkin("")
		return
	}
	for _, skin := range s.library {
		if skin.id == id {
			a.renderAppearanceSkin(skin.path)
			return
		}
	}
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
	token, viewID, variant, raw, remoteRaw, remoteVariant, capeRaw, budget := s.skinID, s.request, s.data.Variant, s.raw, s.remoteRaw, s.remoteVariant, s.capeRaw, a.downloadBudget
	s.data.Busy, s.data.Error = true, ""
	a.workers.Go(func() {
		defer cancel()
		var err error
		sharedPath := path
		libraryChanged := false
		if path != "" {
			var release func()
			release, err = budget.FilePermit(ctx)
			if err == nil {
				raw, err = account.ReadSkinFile(ctx, path)
				if err == nil {
					sharedDir, dirErr := filepath.Abs(appearanceSkinDir())
					sourcePath, sourceErr := filepath.Abs(path)
					if dirErr != nil {
						err = dirErr
					} else if sourceErr != nil {
						err = sourceErr
					} else {
						relative, relativeErr := filepath.Rel(sharedDir, sourcePath)
						inLibrary := relativeErr == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
						if !inLibrary {
							if err = os.MkdirAll(sharedDir, 0o755); err == nil {
								name := filepath.Base(sourcePath)
								target := filepath.Join(sharedDir, name)
								existing, readErr := account.ReadSkinFile(ctx, target)
								if readErr == nil && bytes.Equal(existing, raw) {
									sharedPath = target
								} else {
									sharedPath, err = account.ExportSkin(ctx, sharedDir, name, raw)
								}
								libraryChanged = err == nil
							}
						} else {
							sharedPath = sourcePath
						}
					}
				}
				release()
			}
			if err == nil && path != "" {
				if detected, detectErr := account.DetectSkinVariant(raw); detectErr == nil {
					variant = detected
				}
			}
		} else {
			raw, variant = remoteRaw, remoteVariant
			if variant == "" {
				variant = "classic"
			}
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
			s.raw = raw
			if rendered, renderErr := account.RenderSkin(raw, capeRaw, variant, 360, 420, s.yaw, s.pitch); renderErr == nil {
				s.data.Preview = ui.NewBitmap(rendered)
			}
			if path != "" {
				s.local = true
				s.skinPath = sharedPath
				s.data.FileName = filepath.Base(sharedPath)
				s.data.Variant, s.data.ModelAuto = variant, false
				if libraryChanged {
					s.library = a.scanAppearanceSkins(ctx)
				}
			} else {
				s.local, s.skinPath = false, ""
				s.data.FileName = locales.CurrentSkin()
				s.data.Variant, s.data.ModelAuto = variant, true
			}
			current, _ := s.profile.ActiveSkin()
			s.data.Modified = s.local || !strings.EqualFold(current.Variant, variant)
			info, _ := png.DecodeConfig(bytes.NewReader(raw))
			s.data.CanUpload = info.Width == 64
			s.data.Skins = a.appearanceSkinItems()
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
	s.data.Error, s.data.Previewing = "", false
	s.capeRaw = nil
	if s.data.CapeID == "" {
		if len(s.raw) > 0 {
			if rendered, renderErr := account.RenderSkin(s.raw, nil, s.data.Variant, 360, 420, s.yaw, s.pitch); renderErr == nil {
				s.data.Preview = ui.NewBitmap(rendered)
			}
		}
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
		raw, err := client.TextureCached(ctx, a.cfg.CacheDirectory(), address)
		if err == nil {
			_, err = account.CapePreview(raw)
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.appearance
			if s.request != viewID || s.capeID != token || !s.data.Open || a.ctx.Err() != nil {
				return
			}
			s.data.Previewing = false
			s.capeRaw = raw
			if len(s.raw) > 0 {
				if rendered, renderErr := account.RenderSkin(s.raw, raw, s.data.Variant, 360, 420, s.yaw, s.pitch); renderErr == nil {
					s.data.Preview = ui.NewBitmap(rendered)
				}
			}
			if err != nil {
				s.data.Error = err.Error()
			}
		})
	})
}

func (a *app) rotateAppearance(dx, dy float32) {
	s := &a.appearance
	if !s.data.Open || len(s.raw) == 0 {
		return
	}
	s.yaw += dx * 0.5
	s.pitch += dy * 0.5
	if rendered, err := account.RenderSkin(s.raw, s.capeRaw, s.data.Variant, 360, 420, s.yaw, s.pitch); err == nil {
		s.data.Preview = ui.NewBitmap(rendered)
	}
}

func (a *app) changeAppearanceModel() {
	s := &a.appearance
	if !s.data.Open || s.data.Loading || s.data.Busy || len(s.raw) == 0 {
		return
	}
	if rendered, err := account.RenderSkin(s.raw, s.capeRaw, s.data.Variant, 360, 420, s.yaw, s.pitch); err == nil {
		s.data.Preview = ui.NewBitmap(rendered)
	}
	current, _ := s.profile.ActiveSkin()
	s.data.Modified = s.local || !strings.EqualFold(current.Variant, s.data.Variant)
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
	page.AccountAppearance(c, &a.appearance.data, page.AppearanceActions{Close: a.closeAppearance, Browse: func() { a.openDirectoryPicker(pickerSkin, "") }, OpenFolder: func() { a.openDirectory(appearanceSkinDir()) }, Model: a.changeAppearanceModel, PreviewCape: a.previewAppearanceCape, Rotate: a.rotateAppearance, SelectSkin: a.selectAppearanceSkin, Save: func() { a.openDirectoryPicker(pickerSkinExport, "") }, Upload: func() { a.changeAppearance("skin") }, Reset: func() { a.changeAppearance("reset") }, ApplyCape: func() { a.changeAppearance("cape") }, Drop: a.loadAppearanceSkin})
}
