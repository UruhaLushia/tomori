package page

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type DirectoryActions struct {
	Add                func()
	Edit, Remove, Open func(string)
}

func directorySettings(c *ui.Context, data config.Directories, counts map[string]int, locked bool, actions DirectoryActions) {
	kit.FormGroup(c, locales.GameDirectories(), func() { kit.Ghost(c, locales.AddFolder2(), actions.Add).Height(kit.TabH()).Disabled(locked) }, func() {
		ui.Column(c).FillWidth().Margin(4, -8, 4, -8).Disabled(locked).Children(func() {
			for _, entry := range data.Entries {
				c.Key(entry.Path)
				isDefault := entry.Path == config.DefaultGameDirectory()
				row := ui.Row(c).FillWidth().Height(kit.RowDoubleH()).Padding(0, 0, 0, 8).Gap(12).Radius(kit.RadiusM)
				row.Children(func() {
					kit.IconBadge(c, kit.Folder, 32)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							kit.Line(c, entry.Name, kit.Strong)
							if isDefault {
								kit.Badge(c, locales.Default(), kit.Dim)
							}
						})
						kit.Line(c, kit.MiddleEllipsis(entry.Path, 64), kit.Caption).Tooltip(entry.Path)
					})
					ui.Row(c).Width(kit.W(88)).Shrink(0).Gap(4).Justify(ui.End).AlignItems(ui.Center).Children(func() {
						kit.Label(c, fmt.Sprintf(locales.Instances(), counts[entry.Path]), kit.Caption).SingleLine().Shrink(0)
						kit.ContextMenuButton(c, row, locales.FolderActions()).Size(24, 24)
					})
				})
				kit.ContextMenu(c, row, func(menu *kit.ContextActions) {
					menu.Item(c, "open", locales.Open(), false, func() { actions.Open(entry.Path) })
					if !isDefault {
						menu.Item(c, "edit", locales.Edit(), locked, func() { actions.Edit(entry.Path) })
						menu.Separator(c)
						menu.Item(c, "remove", locales.Remove(), locked, func() { actions.Remove(entry.Path) }).TextColor(kit.Red)
					}
				})
			}
		})
	})
}
