package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

type Game struct {
	memoryValue     float64
	memoryModel     int
	memoryInput     integerSetting
	width, height   integerSetting
	resolution      [2]int
	execution       config.Execution
	executionRead   bool
	executionError  string
	executionLocale string
	commandTimeout  integerSetting
}

func (p *Game) View(c *ui.Context, cfg *config.Config, memory systeminfo.Memory, memoryError string, save func()) {
	ui.Column(c).FillWidth().Shrink(0).Gap(kit.S24).Children(func() { p.settings(c, cfg, memory, memoryError, true, save) })
}

func (p *Game) settings(c *ui.Context, cfg *config.Config, memory systeminfo.Memory, memoryError string, isolation bool, save func()) {
	p.performanceSettings(c, &cfg.Game, memory, memoryError, save)
	p.windowSettings(c, &cfg.Game, save)
	kit.Group(c, locales.MoreOptions(), func() {
		if isolation {
			kit.SettingRow(c, locales.VersionIsolation(), locales.KeepSavesAndSettingsSeparatePerVersion(), func() { kit.Switch(c, &cfg.Game.Isolate).OnChange(save) })
			kit.Divider(c)
		}
		kit.SettingRow(c, locales.ExtraJvmArguments(), "", func() {
			kit.Input(c, &cfg.Game.ExtraJVM).Placeholder("-XX:+UseG1GC").Width(kit.W(260)).OnChange(save)
		})
	})
	p.executionSettings(c, &cfg.Game.Execution, save)
}
