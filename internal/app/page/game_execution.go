package page

import (
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Game) executionSettings(c *ui.Context, cfg *config.Execution, save func()) {
	if !p.executionRead || p.execution != *cfg || p.executionLocale != i18n.Locale() {
		p.execution, p.executionRead, p.executionError = *cfg, true, ""
		p.executionLocale = i18n.Locale()
		if err := cfg.Validate(); err != nil {
			p.executionError = err.Error()
		}
	}
	c.Key("execution-settings")
	kit.Group(c, locales.AdvancedLaunch(), func() {
		kit.SettingRow(c, locales.CustomLaunch(), locales.PreLaunchPostExitCommandsAndEnvironmentVariables(), func() {
			kit.Switch(c, &cfg.Enabled).Label(locales.EnableCustomLaunch()).OnChange(save)
		})
		if !cfg.Enabled {
			return
		}
		kit.FoldMotion(ui.Column(c.Key("execution-fields")).FillWidth()).Children(func() {
			kit.Divider(c)
			ui.Column(c).FillWidth().Padding(12, 4).Gap(12).Children(func() {
				for _, field := range []struct {
					name  string
					value *string
				}{{locales.PreLaunchCommand(), &cfg.Before}, {locales.PostExitCommand(), &cfg.After}, {locales.EnvironmentVariables(), &cfg.Environment}} {
					c.Key(field.value)
					ui.Column(c).FillWidth().Gap(8).Children(func() {
						kit.Label(c, field.name, kit.Body)
						input := kit.TextArea(c, field.value, kit.Mono).FillWidth().Lines(1, 3).Label(field.name).OnChange(save)
						if field.value == &cfg.Environment {
							input.Placeholder("NAME=value").Lines(2, 5)
						}
					})
				}
				if p.executionError != "" {
					kit.ErrorText(c, p.executionError)
				}
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.CommandTimeout(), "", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					p.commandTimeout.input(c, &cfg.TimeoutSeconds, 1, 3600, 82, locales.CommandTimeoutInSeconds(), save)
					kit.Label(c, locales.S(), kit.Body).TextColor(kit.Muted)
				})
			})
		})
	})
}
