package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type PackRestoreData struct {
	Open, Busy, Ready                bool
	Key, Name, Error, Status, Backup string
	Backups                          []modpack.BackupInfo
	Info                             modpack.BackupInfo
	Space                            SpaceData
}

type PackRestoreActions struct {
	Close, Refresh, Apply, Back, OpenBackup func()
	Choose                                  func(modpack.BackupInfo)
}

type PackRestore struct{ list ui.ListState }

func (p *PackRestore) View(c *ui.Context, data *PackRestoreData, actions PackRestoreActions) {
	if !data.Open {
		return
	}
	c.Key("pack-restore:" + data.Key)
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(420)
		if data.Ready || data.Backup != "" {
			panel.Height(320)
		}
		panel.Children(func() {
			kit.DialogHeader(c, locales.RestoreModpack(), actions.Close).Label(locales.CloseModpackRestore())
			kit.Label(c, data.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Backup != "" {
				ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).Gap(12).Children(func() {
					kit.Label(c, locales.RestoreComplete(), kit.Body)
					kit.Label(c, data.Backup, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				})
				kit.DialogFooter(c, func() { kit.Button(c, locales.OpenBackupFolder(), actions.OpenBackup) })
				return
			}
			if data.Busy {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					ui.Spinner(c)
					kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).Wrap().MaxLines(2)
				})
				return
			}
			if data.Ready {
				ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).Gap(12).Children(func() {
					kit.Label(c, packVersion(data.Info.From)+" → "+packVersion(data.Info.To), kit.Body).MaxLines(1).Ellipsis("…")
					label := data.Info.CreatedAt.Local().Format("2006-01-02 15:04") + " · " + fmt.Sprintf(locales.Files3(), data.Info.Files)
					if data.Info.Interrupted {
						label = locales.UpdateInterrupted() + " · " + label
					}
					kit.Label(c, label, kit.Body).TextColor(kit.Muted).Wrap().MaxLines(2)
				})
				SpaceSummary(c, data.Space)
				kit.DialogFooter(c, func() {
					kit.Button(c, locales.Back(), actions.Back)
					kit.Primary(c, locales.Restore2(), actions.Apply).Disabled(data.Space.Blocked())
				})
				return
			}
			if len(data.Backups) == 0 {
				ui.Column(c).Grow(1).FillWidth().Center().Children(func() { kit.Label(c, locales.NoModpackBackups(), kit.Body).TextColor(kit.Muted) })
			} else {
				ui.List(c, &p.list, len(data.Backups)).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any { return data.Backups[i].Name }).Scrollbars(false).Rows(func(row ui.ListRow) {
					item := data.Backups[row.Index]
					c := row.Context
					button := kit.RowButton(c, "", false, func() { actions.Choose(item) }).FillWidth().Padding(12, 8).Gap(8).Disabled(item.Issue != "")
					if button.Hovered() {
						button.Background(kit.SurfaceHi)
					}
					button.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							kit.Label(c, backupVersion(item), kit.Body).MaxLines(1).Ellipsis("…")
							label := item.CreatedAt.Local().Format("2006-01-02 15:04") + " · " + fmt.Sprintf(locales.Files3(), item.Files)
							if item.Issue != "" {
								label = item.Issue
							}
							kit.Label(c, label, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
						})
						ui.Icon(c, kit.ChevronRight).FontSize(kit.IconPx(16)).TextColor(kit.Muted).Shrink(0)
					})
				})
			}
			kit.DialogFooter(c, func() { kit.Button(c, locales.Refresh(), actions.Refresh) })
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}

func backupVersion(info modpack.BackupInfo) string {
	label := packVersion(info.From) + " → " + packVersion(info.To)
	if info.Interrupted {
		label = locales.UpdateInterrupted() + " · " + label
	}
	return label
}
