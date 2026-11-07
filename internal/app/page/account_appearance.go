package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type AppearanceData struct {
	Open, Loading, Busy, Ready                    bool
	Previewing, Modified, CanUpload, ConfirmReset bool
	ModelAuto                                     bool
	Name, FileName, Error, Variant                string
	Preview                                       *ui.Bitmap
	Fallback                                      *ui.Bitmap
	Skins                                         []SkinItem
	Capes                                         []account.Cape
	CapeID, ActiveCape                            string
}

type SkinItem struct {
	ID, Name string
	Preview  *ui.Bitmap
	Selected bool
}

type AppearanceActions struct {
	Close, Browse, OpenFolder, Model, PreviewCape, Save, Upload, Reset, ApplyCape func()
	Rotate                                                                        func(float32, float32)
	SelectSkin                                                                    func(string)
	Drop                                                                          func([]string)
}

type skinPreviewDrag struct {
	dragging bool
	x, y     float32
}

func AccountAppearance(c *ui.Context, d *AppearanceData, actions AppearanceActions) {
	if !d.Open {
		return
	}
	c.Key("account-appearance")
	dialog := kit.Dialog(c, &d.Open, kit.DialogL, func(back, panel ui.Element) {
		panel.MinHeight(0).Children(func() {
			title := locales.SkinAndCape()
			if d.ConfirmReset {
				title = locales.RestoreDefaultSkin()
			}
			kit.DialogHeader(c, title, actions.Close)
			if d.ConfirmReset {
				kit.Label(c, locales.RestoreThisAccountSDefaultSkin(), kit.Body).MaxLines(3).Wrap()
				if d.Error != "" {
					kit.ErrorText(c, d.Error).MaxLines(3).Ellipsis("…")
				}
				kit.DialogFooter(c, func() {
					if d.Busy {
						ui.Spinner(c).Size(16, 16)
					}
					kit.Button(c, locales.Back(), func() { d.ConfirmReset = false }).Disabled(d.Busy)
					kit.Primary(c, locales.Restore2(), actions.Reset).Disabled(d.Busy)
				})
				return
			}
			if !d.Busy && !d.Loading && panel.FileDragOver() {
				panel.Border(1, kit.Accent)
			}
			if paths := panel.DroppedFiles(); len(paths) > 0 {
				actions.Drop(paths)
			}
			if d.Error != "" {
				kit.ErrorText(c, d.Error).MaxLines(3).Ellipsis("…")
			}
			ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Row(c).FillWidth().Gap(24).AlignItems(ui.Stretch).Children(func() {
					appearancePreview(c, d, actions)
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(12).Children(func() {
						appearanceLibrary(c, d, actions)
						appearanceCape(c, d, actions)
					})
				})
			})
			kit.DialogFooter(c, func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					kit.IconButton(c, kit.Folder, false, actions.OpenFolder).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
					kit.Button(c, locales.SaveSkin(), actions.Save).Disabled(d.Busy || d.Preview == nil)
					kit.Button(c, locales.RestoreDefault(), func() { d.ConfirmReset = true }).Disabled(d.Busy || d.Loading || !d.Ready)
				})
				ui.Spacer(c)
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(16, 16).Shrink(0).Center().Children(func() {
						if d.Busy || d.Previewing {
							ui.Spinner(c).Size(16, 16)
						}
					})
					kit.Button(c, locales.ImportSkin(), actions.Browse).Disabled(d.Busy || d.Loading)
					upload := kit.Primary(c, locales.UploadSkin(), actions.Upload).Disabled(d.Busy || d.Loading || !d.Modified || !d.CanUpload)
					if d.Preview != nil && !d.CanUpload {
						upload.Tooltip(locales.UploadA6464Or6432PngSkin())
					}
				})
			}).Wrap()
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}

func appearancePreview(c *ui.Context, d *AppearanceData, actions AppearanceActions) {
	ui.Column(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		height := float32(280)
		preview := ui.Box(c).Width(height * .72).Height(height).Center().Radius(kit.RadiusM).Background(kit.SurfaceHi).Clip()
		state := ui.Local(preview, "skin-preview-drag", func() skinPreviewDrag { return skinPreviewDrag{} })
		preview.HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputPointerDown:
				if ev.Button != 0 {
					return false
				}
				state.dragging, state.x, state.y = true, ev.X, ev.Y
				return true
			case ui.InputPointerMove:
				if !state.dragging || actions.Rotate == nil {
					return false
				}
				actions.Rotate(ev.X-state.x, ev.Y-state.y)
				state.x, state.y = ev.X, ev.Y
				return true
			case ui.InputPointerUp:
				state.dragging = false
				return true
			}
			return false
		}).Children(func() {
			if d.Preview != nil {
				ui.Image(c, d.Preview).Fill().Fit(ui.Contain)
			} else if d.Fallback != nil {
				ui.Image(c, d.Fallback).Size(64, 64).Fit(ui.Contain)
			} else {
				ui.Icon(c, kit.User).FontSize(20).TextColor(kit.Muted)
			}
		})
		kit.SegmentedValues(c, &d.Variant, []kit.SelectOption[string]{{Value: "classic", Label: locales.Classic()}, {Value: "slim", Label: locales.Slim()}}, actions.Model).
			Disabled(d.ModelAuto || d.Busy || d.Loading)
	})
}

func appearanceLibrary(c *ui.Context, d *AppearanceData, actions AppearanceActions) {
	library := kit.Card(c, ui.Column(c).Grow(1).Basis(0).MinHeight(0)).Clip()
	grid := ui.Local(library, "skin-grid", func() ui.GridState { return ui.GridState{} })
	library.Children(func() {
		switch {
		case d.Loading:
			ui.Column(c.Key("skin-loading")).Grow(1).FillWidth().Gap(8).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Spinner(c).Size(16, 16)
				kit.Label(c, locales.LoadingAppearance(), kit.Caption)
			})
		case len(d.Skins) == 0:
			kit.EmptyState(c, kit.Image, locales.NoSkins(), locales.DropASkinFileHere(), nil)
		default:
			grid.Key = func(i int) any { return d.Skins[i].ID }
			ui.GridView(c, grid, len(d.Skins), 88, kit.H(112, kit.Body), func(i int) {
				appearanceSkinTile(c, d.Skins[i], actions.SelectSkin)
			}).Grow(1).FillWidth().MinHeight(0).Padding(6).Scrollbars(false)
		}
	})
}

func appearanceSkinTile(c *ui.Context, skin SkinItem, choose func(string)) {
	tile := ui.ButtonBase(c).Column().Fill().Padding(8, 4).Gap(4).Radius(kit.RadiusM).AlignItems(ui.Center).Justify(ui.Center).Label(skin.Name).OnClick(func() {
		if choose != nil {
			choose(skin.ID)
		}
	})
	kit.ListRowBackground(c, tile, skin.Selected).Children(func() {
		if skin.Preview != nil {
			ui.Image(c, skin.Preview).Size(32, 64).Fit(ui.Contain)
		}
		color := kit.Text
		if skin.Selected {
			color = kit.AccentText
		}
		kit.Line(c, skin.Name, kit.Body).FillWidth().TextAlign(ui.Center).TextColor(color).Tooltip(skin.Name)
	})
}

func appearanceCape(c *ui.Context, d *AppearanceData, actions AppearanceActions) {
	ui.Row(c).FillWidth().Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		options := []kit.SelectOption[string]{{Value: "", Label: locales.HideCape()}}
		label := options[0].Label
		for _, cape := range d.Capes {
			name := cape.Alias
			if name == "" {
				name = cape.ID
			}
			options = append(options, kit.SelectOption[string]{Value: cape.ID, Label: name})
			if d.CapeID == cape.ID {
				label = name
			}
		}
		kit.Label(c, locales.Cape(), kit.Body).TextColor(kit.Muted).Shrink(0)
		kit.Select(c, &d.CapeID, label, options, 110, 210, actions.PreviewCape).Grow(1).MinWidth(0).Label(locales.Cape()).Disabled(d.Busy || d.Loading || !d.Ready)
		kit.Button(c, locales.Apply(), actions.ApplyCape).Label(locales.ApplyCape()).Disabled(d.Busy || !d.Ready || d.CapeID == d.ActiveCape)
	})
}
