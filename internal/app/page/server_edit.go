package page

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

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
