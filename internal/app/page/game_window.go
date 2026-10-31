package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

var resolutionPresets = [][2]int{
	{854, 480}, {1280, 720}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3840, 2160},
}

func (p *Game) windowSettings(c *ui.Context, game *config.Game, save func()) {
	kit.Group(c, locales.Window(), func() {
		kit.SettingRow(c, locales.Resolution(), "", func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				options := make([]kit.SelectOption[[2]int], 0, len(resolutionPresets))
				for _, size := range resolutionPresets {
					options = append(options, kit.SelectOption[[2]int]{Value: size, Label: fmt.Sprintf("%d × %d", size[0], size[1])})
				}
				c.Key("resolution-preset")
				preset := kit.Select(c, &p.resolution, locales.Presets(), options, 76, 156, func() {
					game.Width, game.Height = p.resolution[0], p.resolution[1]
					save()
				}).Label(locales.ResolutionPresets())
				if !preset.Changed() {
					p.resolution = [2]int{game.Width, game.Height}
				}
				c.Key("width")
				p.width.input(c, &game.Width, 320, 16384, 64, locales.WindowWidth(), save)
				kit.Label(c, locales.Message3(), kit.Caption)
				c.Key("height")
				p.height.input(c, &game.Height, 240, 16384, 64, locales.WindowHeight(), save)
			})
		})
		kit.Divider(c)
		kit.FormRow(c, locales.Fullscreen(), "", func() { kit.Switch(c, &game.FullScreen).Label(locales.Fullscreen()).OnChange(save) })
	})
}
