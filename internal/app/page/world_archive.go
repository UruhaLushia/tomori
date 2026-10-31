package page

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type WorldArchiveData struct {
	Open, Busy, Loading                                bool
	Mode, Key, Source, Root, Path, Name, Error, Result string
	Info                                               launcher.WorldSource
	Space                                              SpaceData
	Done, Total                                        int64
	Files                                              int
}

type WorldArchiveActions struct {
	Save, Close, Browse, Open func()
	Drop                      func([]string)
}

func WorldArchive(c *ui.Context, data *WorldArchiveData, actions WorldArchiveActions) {
	if !data.Open {
		return
	}
	c.Key("world-archive")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			title, confirm := locales.ImportWorld(), locales.Import()
			switch data.Mode {
			case "backup":
				title, confirm = locales.BackUpWorld(), locales.CreateBackup()
			case "restore":
				title, confirm = locales.RestoreWorld(), locales.RestoreAsANewWorld()
			}
			kit.DialogHeader(c, title, actions.Close)
			kit.Label(c, data.Source, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Mode != "backup" {
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					label := data.Path
					if label == "" {
						label = locales.ChooseWorldArchive()
						if data.Mode == "directory" {
							label = locales.ChooseWorldFolder()
						}
					}
					kit.Label(c, label, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(2).Ellipsis("…").Wrap().Tooltip(data.Path)
					kit.Button(c, locales.Browse(), actions.Browse).Disabled(data.Busy)
				})
				if !data.Busy {
					if panel.FileDragOver() {
						panel.Border(1, kit.Accent)
					}
					if paths := panel.DroppedFiles(); len(paths) > 0 {
						actions.Drop(paths)
					}
				}
			}
			if data.Loading {
				ui.Box(c).FillWidth().Height(64).Center().Children(func() { ui.Spinner(c) })
			} else if data.Info.Path != "" {
				if data.Mode == "backup" {
					kit.Label(c, data.Info.World.Name, kit.Body).MaxLines(2).Ellipsis("…").Wrap()
					kit.Label(c, filepath.Join(data.Root, "backups", "worlds"), kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				} else {
					kit.Input(c, &data.Name).FillWidth().Placeholder(locales.WorldName()).Label(locales.WorldName()).Disabled(data.Busy).OnSubmit(actions.Save)
				}
				metadata := fmt.Sprintf(locales.Files(), data.Info.Files, storage.Size(data.Info.Bytes))
				if data.Info.World.Version != "" {
					metadata = data.Info.World.Version + " · " + metadata
				}
				kit.Label(c, metadata, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				if data.Result == "" {
					SpaceSummary(c, data.Space)
				}
			}
			if data.Result != "" {
				kit.Label(c, data.Result, kit.Body).TextColor(kit.Muted).MaxLines(2).Ellipsis("…").Wrap()
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Busy {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					kit.Label(c, fmt.Sprintf(locales.Files(), data.Files, storage.Size(uint64(max(0, data.Done)))), kit.Body).TextColor(kit.Muted)
				})
				if data.Total > 0 {
					kit.Progress(c, min(1, float64(data.Done)/float64(data.Total))).FillWidth().Height(4)
				}
			}
			kit.DialogFooter(c, func() {
				if data.Result != "" {
					kit.Button(c, locales.OpenFolder(), actions.Open)
				} else {
					kit.Primary(c, confirm, actions.Save).Disabled(data.Busy || data.Loading || data.Info.Path == "" || data.Space.Blocked() || data.Mode != "backup" && strings.TrimSpace(data.Name) == "")
				}
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
