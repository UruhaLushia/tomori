package page

import (
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
)

type PackRecoveryData struct {
	Open, Busy bool
	Error      string
	Entries    []launcher.PackRecovery
}

type PackRecoveryActions struct {
	Close, Refresh func()
	Choose         func(launcher.PackRecovery)
	OpenDirectory  func(string)
}

type PackRecovery struct{ list ui.ListState }

func (p *PackRecovery) View(c *ui.Context, data *PackRecoveryData, actions PackRecoveryActions) {
	if !data.Open {
		return
	}
	c.Key("pack-recovery")
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(420).Children(func() {
			kit.DialogHeader(c, locales.InterruptedUpdates(), actions.Close)
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Wrap()
			}
			if data.Busy {
				ui.Box(c).Grow(1).FillWidth().Center().Children(func() { ui.Spinner(c) })
			} else if len(data.Entries) == 0 {
				ui.Box(c).Grow(1).FillWidth().Center().Children(func() {
					kit.Label(c, locales.NoInterruptedUpdates(), kit.Body).TextColor(kit.Muted)
				})
			} else {
				ui.List(c, &p.list, len(data.Entries)).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).
					ItemKey(func(i int) any { return data.Entries[i].Info.Name }).Scrollbars(false).Rows(func(row ui.ListRow) {
					item, c := data.Entries[row.Index], row.Context
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						button := kit.RowButton(c, "", false, func() { actions.Choose(item) }).Grow(1).MinWidth(0).Padding(12, 8).Disabled(item.Info.Issue != "")
						if button.Hovered() {
							button.Background(kit.SurfaceHi)
						}
						button.Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
								kit.Label(c, item.Instance.Name, kit.Body).MaxLines(1).Ellipsis("…")
								label := packVersion(item.Info.From) + " → " + packVersion(item.Info.To)
								if item.Info.Issue != "" {
									label = item.Info.Issue
								}
								kit.Label(c, label, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
								kit.Label(c, item.Content, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
							})
							ui.Icon(c, kit.ChevronRight).FontSize(16).TextColor(kit.Muted).Shrink(0)
						})
						kit.IconButton(c, kit.Folder, false, func() { actions.OpenDirectory(item.Instance.InstanceDir) }).Label(locales.OpenInstanceFolder())
					})
				})
			}
			kit.DialogFooter(c, func() { kit.Button(c, locales.Refresh(), actions.Refresh).Disabled(data.Busy) })
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
