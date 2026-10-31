package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldEditData struct {
	Open, Busy                                           bool
	Mode, Key, Folder, Name, Source, Path, Target, Error string
	Targets                                              []kit.SelectOption[string]
	Bytes, TotalBytes                                    int64
	Files                                                int
}

func WorldEdit(c *ui.Context, data *WorldEditData, save, close func()) {
	if !data.Open {
		return
	}
	c.Key("world-edit")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			title, confirm := locales.RenameWorld(), locales.Save()
			switch data.Mode {
			case "copy":
				title, confirm = locales.CopyWorld(), locales.CreateCopy()
			case "delete":
				title, confirm = locales.DeleteWorld(), locales.MoveToTrash()
			}
			kit.DialogHeader(c, title, close)
			kit.Label(c, data.Source, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Mode == "delete" {
				kit.Label(c, data.Name, kit.Body).MaxLines(2).Ellipsis("…").Wrap()
			} else {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.Label(c, locales.WorldName(), kit.Body).TextColor(kit.Muted)
					kit.Input(c, &data.Name).FillWidth().Label(locales.WorldName()).Disabled(data.Busy).OnSubmit(save)
				})
			}
			if data.Mode == "copy" {
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.Label(c, locales.TargetInstance(), kit.Body).TextColor(kit.Muted)
					label := ""
					for _, target := range data.Targets {
						if target.Value == data.Target {
							label = target.Label
							break
						}
					}
					kit.Select(c, &data.Target, label, data.Targets, 0, 420, nil).FillWidth().MinWidth(0).Shrink(1).Disabled(data.Busy).Label(locales.TargetInstance())
				})
			}
			kit.Label(c, data.Path, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…").Tooltip(data.Path)
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Busy && data.Mode == "copy" {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					label := locales.Preparing()
					if data.Files > 0 || data.Bytes > 0 {
						label = fmt.Sprintf(locales.CopiedFilesMiB(), data.Files, float64(data.Bytes)/(1<<20))
					}
					kit.Label(c, label, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				})
				if data.TotalBytes > 0 {
					kit.Progress(c, min(1, float64(data.Bytes)/float64(data.TotalBytes))).FillWidth().Height(4)
				}
			}
			kit.DialogFooter(c, func() {
				if data.Busy && data.Mode != "copy" {
					ui.Spinner(c).Size(16, 16)
				}
				disabled := data.Busy || data.Mode != "delete" && strings.TrimSpace(data.Name) == "" || data.Mode == "copy" && data.Target == ""
				if data.Mode == "delete" {
					kit.Danger(c, confirm, save).Disabled(disabled)
				} else {
					kit.Primary(c, confirm, save).Disabled(disabled)
				}
			})
		})
	})
	if dialog.Dismissed() {
		close()
	}
}
