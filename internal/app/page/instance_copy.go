package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/locales"
)

type InstanceCopyData struct {
	Open, Busy, Settings bool
	Key, Source, Error   string
	Request              launcher.InstanceCopy
	Directories          []config.GameDirectory
	Bytes, TotalBytes    int64
	Files, TotalFiles    int
}

func InstanceCopy(c *ui.Context, data *InstanceCopyData, save, close func()) {
	c.Key("instance-copy")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Height(490)
		panel.Children(func() {
			kit.DialogHeader(c, locales.CopyInstance(), close)
			ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(12).Children(func() {
					kit.Label(c, data.Source, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					kit.Input(c, &data.Request.Name).FillWidth().Placeholder(locales.InstanceName()).Label(locales.InstanceName()).Disabled(data.Busy).OnSubmit(save)
					ui.Row(c).Gap(12).FillWidth().AlignItems(ui.Center).Children(func() {
						kit.Label(c, locales.InstallLocation(), kit.Body).TextColor(kit.Muted)
						var options []kit.SelectOption[string]
						label := ""
						for _, directory := range data.Directories {
							options = append(options, kit.SelectOption[string]{Value: directory.Path, Label: directory.Name + " · " + directory.Path})
							if directory.Path == data.Request.Directory {
								label = directory.Name
							}
						}
						kit.Select(c, &data.Request.Directory, label, options, 240, 420, nil).Grow(1).MinWidth(0).Disabled(data.Busy).Label(locales.InstallLocation())
					})
					kit.Label(c, data.Request.Directory, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					kit.Card(c, ui.Column(c).Padding(12).Gap(12)).Children(func() {
						kit.Label(c, locales.Include(), kit.Body)
						for _, option := range []struct {
							label string
							value *bool
						}{
							{locales.WorldsAndBackups(), &data.Request.Worlds}, {locales.Screenshots(), &data.Request.Screenshots}, {locales.LogsAndCrashReports(), &data.Request.Logs}, {locales.OtherPersonalData(), &data.Request.Personal}, {locales.InstanceSettings2(), &data.Settings},
						} {
							c.Key(option.label)
							ui.Checkbox(c, option.value, option.label).Disabled(data.Busy)
						}
					})
				})
			})
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Busy {
				ui.Row(c).Gap(8).FillWidth().AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					kit.Label(c, fmt.Sprintf(locales.CopiedFilesMiB(), data.Files, float64(data.Bytes)/(1<<20)), kit.Body).TextColor(kit.Muted).Grow(1).MaxLines(1).Ellipsis("…")
				})
			}
			kit.DialogFooter(c, func() {
				kit.Primary(c, locales.CreateCopy(), save).Disabled(data.Busy || strings.TrimSpace(data.Request.Name) == "" || data.Request.Directory == "")
			})
		})
	})
	if dialog.Dismissed() {
		close()
	}
}
