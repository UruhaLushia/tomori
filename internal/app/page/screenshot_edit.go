package page

import (
	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type ScreenshotEditData struct {
	Open, Busy                                bool
	Mode, Key, Root, Directory, Error, Result string
	File                                      instancecontent.File
}

type ScreenshotEditActions struct{ Close, Browse, Save, Open func() }

func ScreenshotEdit(c *ui.Context, data *ScreenshotEditData, actions ScreenshotEditActions) {
	if !data.Open {
		return
	}
	c.Key("screenshot-edit")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			title := locales.ExportScreenshot()
			if data.Mode == "delete" {
				title = locales.DeleteScreenshot()
			}
			kit.DialogHeader(c, title, actions.Close)
			kit.Label(c, data.File.Name, kit.Body).FillWidth().Wrap().MaxLines(2).Ellipsis("…")
			kit.Label(c, data.File.Modified.Local().Format("2006-01-02 15:04")+" · "+storage.Size(uint64(max(0, data.File.Size))), kit.Body).TextColor(kit.Muted)
			if data.Error != "" {
				kit.ErrorText(c, data.Error).Wrap().MaxLines(3).Ellipsis("…")
			}
			if data.Result != "" {
				kit.Label(c, data.Result, kit.Body).TextColor(kit.Muted).FillWidth().Wrap().MaxLines(2).Ellipsis("…")
				kit.DialogFooter(c, func() { kit.Primary(c, locales.OpenExportFolder(), actions.Open) })
				return
			}
			if data.Mode == "delete" {
				kit.Label(c, locales.MoveThisScreenshotToTrash(), kit.Body).Wrap()
			} else {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					kit.Label(c, data.Directory, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…").Tooltip(data.Directory)
					kit.Button(c, locales.Browse(), actions.Browse).Disabled(data.Busy).Label(locales.ExportFolder())
				})
			}
			kit.DialogFooter(c, func() {
				if data.Busy {
					ui.Spinner(c).Size(16, 16)
				}
				if data.Mode == "delete" {
					kit.Danger(c, locales.MoveToTrash(), actions.Save).Disabled(data.Busy)
				} else {
					kit.Primary(c, locales.Export(), actions.Save).Disabled(data.Busy || data.Directory == "")
				}
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
