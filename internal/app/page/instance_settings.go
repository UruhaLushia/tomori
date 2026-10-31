package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

type InstanceSettingsData struct {
	Open, Enabled, Isolation bool
	Name, Key                string
	Config                   config.Config
	Inherited                config.Config
	Icon                     *ui.Bitmap
}

type InstanceSettings struct {
	game Game
	tab  int
}

func (p *InstanceSettings) View(c *ui.Context, data *InstanceSettingsData, memory systeminfo.Memory, memoryError string, java JavaData, save, close func()) {
	if !data.Open {
		return
	}
	c.Key("instance-settings:" + data.Key)
	dialog := kit.Dialog(c, &data.Open, kit.DialogL, func(back, panel ui.Element) {
		panel.Height(640)
		panel.Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.InstanceIcon(c, data.Icon, 36)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					kit.Label(c, locales.InstanceSettings(), kit.Body)
					kit.Label(c, data.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				})
				kit.IconButton(c, kit.Close, false, close).Label(locales.CloseInstanceSettings())
			})
			kit.Group(c, locales.InstanceSettings(), func() {
				hint := locales.UseGlobalSettings()
				if data.Enabled {
					hint = ""
				}
				kit.SettingRow(c, locales.CustomSettings(), hint, func() { kit.Switch(c, &data.Enabled).Label(locales.UseCustomSettings()).OnChange(save) })
			})
			ui.Row(c).FillWidth().Children(func() {
				c.Key("instance-settings-tabs")
				kit.Segmented(c, &p.tab, locales.Game(), "Java")
			})
			c.Key(p.tab)
			cfg := &data.Config
			if !data.Enabled {
				cfg = &data.Inherited
			}
			kit.PageMotion(c, kit.Scroll(c, func() {
				if p.tab == 1 {
					javaSettings(c, cfg, java, save)
				} else {
					p.game.settings(c, cfg, memory, memoryError, data.Isolation, save)
				}
			})).Disabled(!data.Enabled)
		})
	})
	if dialog.Dismissed() {
		close()
	}
}
