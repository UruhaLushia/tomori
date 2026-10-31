package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldBackupsData struct {
	Open, Busy, Loading, Ready, Delete                           bool
	Key, Source, Root, Mode, Target, Name, Error, Result, Backup string
	Backups                                                      []launcher.WorldBackup
	Selected                                                     launcher.WorldBackup
	Info, TargetInfo                                             launcher.WorldSource
	Targets                                                      []kit.SelectOption[string]
	Space                                                        SpaceData
	Done, Total                                                  int64
	Files                                                        int
}

type WorldBackupsActions struct {
	Close, Refresh, Back, Changed, Save, Open, OpenResult func()
	Choose, Delete                                        func(launcher.WorldBackup)
}

type WorldBackups struct{ list ui.ListState }

func (p *WorldBackups) View(c *ui.Context, data *WorldBackupsData, actions WorldBackupsActions) {
	if !data.Open {
		return
	}
	c.Key("world-backups:" + data.Key)
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(520).Children(func() {
			kit.DialogHeader(c, locales.WorldBackups(), actions.Close)
			kit.Label(c, data.Source, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Wrap().Ellipsis("…")
			}
			if data.Result != "" {
				ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).Gap(12).Children(func() {
					kit.Label(c, locales.RestoreComplete(), kit.Body)
					kit.Label(c, data.Result, kit.Body).TextColor(kit.Muted).FillWidth().Wrap().MaxLines(2)
					if data.Backup != "" {
						kit.Label(c, locales.OriginalWorldBackedUp(), kit.Body).TextColor(kit.Muted)
					}
				})
				kit.DialogFooter(c, func() {
					kit.Button(c, locales.Back(), actions.Back)
					kit.Primary(c, locales.OpenFolder(), actions.OpenResult)
				})
				return
			}
			if data.Busy || data.Loading {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					ui.Spinner(c)
					if data.Busy {
						kit.Label(c, fmt.Sprintf(locales.Files(), data.Files, storage.Size(uint64(max(0, data.Done)))), kit.Body).TextColor(kit.Muted)
					}
					if data.Total > 0 {
						kit.Progress(c, min(1, float64(data.Done)/float64(data.Total))).FillWidth().Height(4)
					}
				})
				return
			}
			if data.Ready {
				ui.Scroll(c).Scrollbars(false).FillWidth().Grow(1).MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0).Gap(12).Children(func() {
						kit.Label(c, data.Selected.Name, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
						if data.Delete {
							kit.Label(c, locales.MoveThisBackupToTrash(), kit.Body).Wrap()
							return
						}
						if data.Info.Path == "" {
							return
						}
						kit.Label(c, data.Info.World.Name, kit.Body).Wrap().MaxLines(2)
						meta := fmt.Sprintf(locales.Files(), data.Info.Files, storage.Size(data.Info.Bytes))
						if data.Info.World.Version != "" {
							meta = data.Info.World.Version + " · " + meta
						}
						kit.Label(c, meta, kit.Body).TextColor(kit.Muted).Wrap().MaxLines(2)
						mode := locales.RestoreAsANewWorld()
						if data.Mode == "replace" {
							mode = locales.ReplaceExistingWorld()
						}
						options := []kit.SelectOption[string]{{Value: "new", Label: locales.RestoreAsANewWorld()}}
						if len(data.Targets) > 0 {
							options = append(options, kit.SelectOption[string]{Value: "replace", Label: locales.ReplaceExistingWorld()})
						}
						kit.Select(c, &data.Mode, mode, options, 200, 240, actions.Changed).FillWidth().MinWidth(0).Label(locales.RestoreMode())
						if data.Mode == "replace" {
							target := ""
							for _, option := range data.Targets {
								if option.Value == data.Target {
									target = option.Label
									break
								}
							}
							kit.Select(c, &data.Target, target, data.Targets, 200, 280, actions.Changed).FillWidth().MinWidth(0).Label(locales.TargetWorld())
							kit.Label(c, locales.TheCurrentWorldWillBeBackedUpAutomaticallyBeforeReplacement(), kit.Body).TextColor(kit.Muted).Wrap()
						} else {
							kit.Input(c, &data.Name).FillWidth().Placeholder(locales.WorldName()).Label(locales.WorldName()).OnSubmit(actions.Save)
						}
						SpaceSummary(c, data.Space)
					})
				})
				kit.DialogFooter(c, func() {
					kit.Button(c, locales.Back(), actions.Back)
					if !data.Delete {
						kit.IconButton(c, kit.Trash, false, func() { actions.Delete(data.Selected) }).Label(locales.MoveToTrash())
					}
					label := locales.Restore2()
					if data.Delete {
						label = locales.MoveToTrash()
					}
					kit.Primary(c, label, actions.Save).Disabled(!data.Delete && (data.Info.Path == "" || data.Space.Blocked() || data.Mode == "new" && strings.TrimSpace(data.Name) == "" || data.Mode == "replace" && data.TargetInfo.Path == ""))
				})
				return
			}
			if len(data.Backups) == 0 {
				ui.Box(c).Grow(1).FillWidth().Center().Children(func() { kit.Label(c, locales.NoWorldBackups(), kit.Body).TextColor(kit.Muted) })
			} else {
				ui.List(c, &p.list, len(data.Backups)).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any { return data.Backups[i].Name }).Scrollbars(false).Rows(func(row ui.ListRow) {
					c, item := row.Context, data.Backups[row.Index]
					button := kit.RowButton(c, "", false, func() { actions.Choose(item) }).FillWidth().Padding(12, 8).Gap(8)
					if button.Hovered() {
						button.Background(kit.SurfaceHi)
					}
					button.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							kit.Label(c, item.Name, kit.Body).MaxLines(1).Ellipsis("…")
							kit.Label(c, item.Modified.Local().Format("2006-01-02 15:04")+" · "+storage.Size(uint64(max(0, item.Size))), kit.Body).TextColor(kit.Muted).SingleLine()
						})
						ui.Icon(c, kit.ChevronRight).FontSize(kit.IconPx(12)).TextColor(kit.Muted).Shrink(0)
					})
					kit.ContextMenu(c, button, func(menu *kit.ContextActions) {
						menu.Item(c, "restore", locales.Restore2(), false, func() { actions.Choose(item) })
						menu.Item(c, "trash", locales.MoveToTrash(), false, func() { actions.Delete(item) })
					})
				})
			}
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.OpenBackupFolder(), actions.Open).Disabled(len(data.Backups) == 0)
				kit.Button(c, locales.Refresh(), actions.Refresh)
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
