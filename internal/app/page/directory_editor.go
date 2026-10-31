package page

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type DirectoryEditorData struct {
	Open, Saving             bool
	Title, Name, Path, Error string
}

type DirectoryEditorActions struct {
	Browse, Save, Cancel func()
}

func DirectoryEditor(c *ui.Context, data *DirectoryEditorData, actions DirectoryEditorActions) {
	c.Key("directory-editor")
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, data.Title, actions.Cancel)
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				kit.Label(c, locales.Name(), kit.Body).TextColor(kit.Muted)
				c.Key("directory-name")
				kit.Input(c, &data.Name).FillWidth().Placeholder(locales.DirectoryName()).Disabled(data.Saving)
			})
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				kit.Label(c, locales.Path(), kit.Body).TextColor(kit.Muted)
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					c.Key("directory-path")
					kit.Input(c, &data.Path).Grow(1).MinWidth(0).Label(locales.DirectoryPath()).Disabled(data.Saving)
					kit.Button(c, locales.Browse(), actions.Browse).Disabled(data.Saving)
				})
			})
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			kit.DialogFooter(c, func() {
				if data.Saving {
					ui.Spinner(c)
				}
				ui.Spacer(c)
				kit.Primary(c, locales.Save(), actions.Save).Disabled(data.Saving || strings.TrimSpace(data.Path) == "")
			})
		})
	})
	if dialog.Dismissed() {
		actions.Cancel()
	}
}
