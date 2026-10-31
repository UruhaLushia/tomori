package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

type ServerResult struct {
	Value         serverstatus.Status
	Done, Loading bool
	Error         string
}

type ServersData struct {
	Key, Root, Error       string
	Loading, Ready, Locked bool
	Protocol               int
	List                   instancecontent.ServerList
	Status                 map[int]ServerResult
	Edit                   ServerEditData
}

type ServerEditData struct {
	Open, Busy, Remove, New bool
	Name, Address, Error    string
	Textures                int
}

type ServersActions struct {
	Refresh func()
	Query   func(int)
	Edit    func(int, bool)
}

func (p *Instances) servers(c *ui.Context, data *ServersData, actions ServersActions) {
	kit.Toolbar(c, func() {
		kit.Label(c, fmt.Sprintf(locales.Servers2(), len(data.List.Entries)), kit.Caption).Grow(1)
		kit.IconButton(c, kit.Refresh, false, actions.Refresh).Label(locales.RefreshServers()).Tooltip(locales.Refresh()).Disabled(data.Loading || data.Edit.Open)
		kit.IconLabel(c, kit.Plus, locales.Add(), func() { actions.Edit(-1, false) }).Label(locales.AddServer()).Disabled(data.Locked || !data.Ready)
	})
	if data.Error != "" {
		kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
	}
	if data.Loading {
		kit.Loading(c)
		return
	}
	if len(data.List.Entries) == 0 {
		kit.EmptyState(c, nil, locales.NoServers(), "", nil)
		return
	}
	c.Key("server-results")
	list := kit.PresenceMotion(ui.List(c, &p.serverList, len(data.List.Entries)).Grow(1).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0)).ItemKey(func(i int) any { return fmt.Sprintf("%x:%d", data.List.Revision[:8], i) })
	mediaViewport(c, list)
	list.Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		i := row.Index
		entry := data.List.Entries[i]
		result := data.Status[i]
		locked := data.Locked || !data.Ready
		mismatch := result.Done && result.Error == "" && !result.Value.Compatible(data.Protocol)
		panel := kit.RowButton(c, entry.Name, false, nil).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
		if panel.InView() && !result.Done && !result.Loading && actions.Query != nil {
			actions.Query(i)
		}
		panel.Children(func() {
			ui.Box(c).Size(32, 32).Shrink(0).Children(func() {
				kit.IconBadge(c, kit.Grid, 32)
				if result.Done && result.Error == "" {
					ui.Box(c).Absolute().Right(0).Bottom(0).Size(8, 8).Radius(kit.RadiusFull).Background(kit.Green)
				}
			})
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				kit.Line(c, entry.Name, kit.Strong)
				kit.Line(c, entry.Address, kit.Caption)
			})
			ui.Row(c).Width(kit.W(104)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if panel.Hovered() || panel.FocusVisible() {
					kit.ContextMenuButton(c, panel, locales.ServerActions())
				} else if !result.Done {
					kit.Line(c, locales.Searching(), kit.Caption)
				} else if result.Error != "" {
					kit.Line(c, locales.Offline(), kit.Caption)
				} else {
					ui.Column(c).AlignItems(ui.End).Children(func() {
						label, color := fmt.Sprintf("%d ms", result.Value.Latency.Milliseconds()), kit.Muted
						if mismatch {
							label, color = locales.IncompatibleVersion(), kit.Red
						}
						kit.Line(c, label, kit.Caption).TextColor(color)
						kit.Line(c, result.Value.Players(), kit.Caption)
					})
				}
			})
		})
		services := c.Services()
		kit.ContextMenu(c, panel, func(menu *kit.ContextActions) {
			menu.Item(c, "edit", locales.Edit(), locked, func() { actions.Edit(i, false) })
			menu.Item(c, "copy", locales.CopyAddress(), false, func() { services.WriteClipboard(entry.Address) })
			menu.Separator(c)
			menu.Item(c, "delete", locales.Delete(), locked, func() { actions.Edit(i, true) }, kit.DangerTone)
		})
	})
}

func ServerEdit(c *ui.Context, data *ServerEditData, save, close func()) {
	if !data.Open {
		return
	}
	c.Key("server-edit")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			title := locales.EditServer()
			if data.New {
				title = locales.AddServer()
			} else if data.Remove {
				title = locales.DeleteServer()
			}
			kit.DialogHeader(c, title, close)
			if data.Remove {
				kit.Label(c, data.Name, kit.Body).MaxLines(2).Ellipsis("…")
				kit.Label(c, data.Address, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			} else {
				for _, field := range []struct {
					label string
					value *string
				}{{locales.ServerName(), &data.Name}, {locales.ServerAddress(), &data.Address}} {
					c.Key(field.label)
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						kit.Label(c, field.label, kit.Body).TextColor(kit.Muted)
						kit.Input(c, field.value).FillWidth().Label(field.label).Disabled(data.Busy).OnSubmit(save)
					})
				}
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					kit.Label(c, locales.ServerResourcePack(), kit.Body).Grow(1).MinWidth(0)
					options := []kit.SelectOption[int]{{Value: 0, Label: locales.Ask()}, {Value: 1, Label: locales.Enable()}, {Value: 2, Label: locales.Disable()}}
					kit.Select(c, &data.Textures, options[max(0, min(2, data.Textures))].Label, options, 90, 140, nil).Label(locales.ServerResourcePack()).Disabled(data.Busy)
				})
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			kit.DialogFooter(c, func() {
				if data.Busy {
					ui.Spinner(c).Size(16, 16)
				}
				if data.Remove {
					kit.Danger(c, locales.Delete(), save).Disabled(data.Busy)
				} else {
					kit.Primary(c, locales.Save(), save).Disabled(data.Busy || strings.TrimSpace(data.Name) == "" || strings.TrimSpace(data.Address) == "")
				}
			})
		})
	})
	if dialog.Dismissed() {
		close()
	}
}
