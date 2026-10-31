package page

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type InstanceEditData struct {
	Open, Saving, Changed         bool
	Mode, Key, Name, Path, Source string
	Error, Notice                 string
	Icon                          *ui.Bitmap
}

type InstanceEditActions struct {
	Save, Close, Browse, Reset func()
}

func InstanceEdit(c *ui.Context, data *InstanceEditData, actions InstanceEditActions) {
	c.Key("instance-edit")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			title, confirm := locales.RenameInstance(), locales.Save()
			switch data.Mode {
			case "icon":
				title = locales.ChangeIcon()
			case "delete":
				title, confirm = locales.DeleteInstance(), locales.MoveToTrash()
			}
			kit.DialogHeader(c, title, actions.Close).Disabled(data.Saving)
			switch data.Mode {
			case "rename":
				c.Key("instance-name")
				kit.Input(c, &data.Name).FillWidth().Label(locales.InstanceName()).Disabled(data.Saving).OnSubmit(actions.Save)
			case "icon":
				ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
					kit.InstanceIcon(c, data.Icon, 64)
					ui.Column(c).Grow(1).MinWidth(0).Gap(8).Children(func() {
						kit.Label(c, data.Name, kit.Body).MaxLines(1).Ellipsis("…")
						ui.Row(c).Gap(8).Children(func() {
							kit.Button(c, locales.ChooseImage(), actions.Browse).Disabled(data.Saving)
							kit.Button(c, locales.Restore(), actions.Reset).Disabled(data.Saving)
						})
					})
				})
			default:
				kit.Label(c, data.Name, kit.Body).MaxLines(2).Wrap()
				kit.Label(c, locales.InstanceFilesWillBeMovedToTheTrashSharedAssetsAndLibrariesWillBeKept(), kit.Body).Wrap().TextColor(kit.Muted)
			}
			if data.Mode != "icon" {
				kit.Label(c, data.Path, kit.Body).TextColor(kit.Muted).Wrap().MaxLines(3)
			}
			if data.Notice != "" {
				kit.Label(c, data.Notice, kit.Body).TextColor(kit.Muted).Wrap()
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			kit.DialogFooter(c, func() {
				if data.Saving {
					ui.Spinner(c)
				}
				ui.Spacer(c)
				disabled := data.Saving || data.Mode == "rename" && strings.TrimSpace(data.Name) == "" || data.Mode == "icon" && !data.Changed
				if data.Mode == "delete" {
					kit.Danger(c, confirm, actions.Save).Disabled(disabled)
				} else {
					kit.Primary(c, confirm, actions.Save).Disabled(disabled)
				}
			})
		})
	})
	if dialog.Dismissed() {
		if data.Saving {
			data.Open = true
		} else {
			actions.Close()
		}
	}
}
