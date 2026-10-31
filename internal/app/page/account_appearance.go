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
	Name, FileName, Error, Variant                string
	Side                                          int
	Skin                                          [2]*ui.Bitmap
	Fallback, Cape                                *ui.Bitmap
	Capes                                         []account.Cape
	CapeID, ActiveCape                            string
}

type AppearanceActions struct {
	Close, Refresh, Browse, Model, PreviewCape, Save, Upload, Reset, ApplyCape func()
	Drop                                                                       func([]string)
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
			if paths := panel.DroppedFiles(); len(paths) > 0 {
				actions.Drop(paths)
			}
			if d.Loading {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					kit.Label(c, locales.LoadingAppearance(), kit.Body).TextColor(kit.Muted)
				})
			}
			if d.Error != "" {
				kit.ErrorText(c, d.Error).MaxLines(3).Ellipsis("…")
			}
			ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
				width, _ := c.Size()
				body := ui.Row(c).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0).Gap(24).AlignItems(ui.Center)
				if width < 560 {
					body.Column().Gap(12)
				}
				body.Children(func() {
					ui.Column(c).Gap(8).Shrink(0).AlignItems(ui.Center).Children(func() {
						height := float32(240)
						if width < 560 {
							height = 152
						}
						ui.Row(c).Gap(16).Height(height).AlignItems(ui.Center).Children(func() {
							preview := ui.Box(c).Size(height/2, height).Center().Radius(kit.RadiusM).Background(kit.SurfaceHi)
							preview.Children(func() {
								if bitmap := d.Skin[max(0, min(1, d.Side))]; bitmap != nil {
									ui.Image(c, bitmap).Fill().Fit(ui.Contain)
								} else if d.Fallback != nil {
									ui.Image(c, d.Fallback).Size(64, 64).Fit(ui.Contain)
								} else {
									ui.Icon(c, kit.User).FontSize(kit.IconPx(20)).TextColor(kit.Muted)
								}
							})
							if d.Cape != nil {
								ui.Image(c, d.Cape).Size(height/3, height*2/3).Fit(ui.Contain)
							}
						})
						kit.Tabs(c, &d.Side, []string{locales.Front(), locales.Back3()}, nil).Label(locales.SkinView())
					})
					ui.Column(c).Grow(1).MinWidth(0).FillWidth().Gap(12).Children(func() {
						kit.Label(c, d.Name, kit.Body).MaxLines(1).Ellipsis("…")
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							kit.Button(c, locales.ChooseSkin(), actions.Browse).Disabled(d.Busy || d.Loading)
							kit.Label(c, d.FileName, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…").Tooltip(d.FileName)
						})
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							kit.Label(c, locales.Model(), kit.Body).Grow(1).MinWidth(0)
							options := []kit.SelectOption[string]{{Value: "classic", Label: locales.Classic()}, {Value: "slim", Label: locales.Slim()}}
							label := options[0].Label
							if d.Variant == "slim" {
								label = options[1].Label
							}
							kit.Select(c, &d.Variant, label, options, 108, 150, actions.Model).Label(locales.SkinModel()).Disabled(d.Busy || d.Loading)
						})
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							kit.Button(c, locales.SaveSkin(), actions.Save).Disabled(d.Busy || d.Skin[0] == nil)
							kit.Button(c, locales.RestoreDefault(), func() { d.ConfirmReset = true }).Disabled(d.Busy || d.Loading || !d.Ready)
						})
						kit.Divider(c)
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
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
							kit.Select(c, &d.CapeID, label, options, 110, 210, actions.PreviewCape).Grow(1).MinWidth(0).Label(locales.Cape()).Disabled(d.Busy || d.Loading || !d.Ready)
							kit.Button(c, locales.Apply(), actions.ApplyCape).Label(locales.ApplyCape()).Disabled(d.Busy || !d.Ready || d.CapeID == d.ActiveCape)
						})
					})
				})
			})
			kit.DialogFooter(c, func() {
				if d.Busy || d.Previewing {
					ui.Spinner(c).Size(16, 16)
				}
				kit.IconButton(c, kit.Refresh, false, actions.Refresh).Label(locales.ReloadAppearance()).Tooltip(locales.Reload2()).Disabled(d.Busy || d.Loading)
				upload := kit.Primary(c, locales.UploadSkin(), actions.Upload).Disabled(d.Busy || d.Loading || !d.Modified || !d.CanUpload)
				if d.Skin[0] != nil && !d.CanUpload {
					upload.Tooltip(locales.UploadA6464Or6432PngSkin())
				}
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
