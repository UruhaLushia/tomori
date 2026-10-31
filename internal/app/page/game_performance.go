package page

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/systeminfo"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Game) performanceSettings(c *ui.Context, game *config.Game, memory systeminfo.Memory, memoryError string, save func()) {
	kit.Group(c, locales.Performance(), func() {
		kit.SettingRow(c, locales.AutomaticMemoryAllocation(), "", func() {
			kit.Switch(c, &game.MemoryAuto).Label(locales.AllocateMemoryAutomatically()).OnChange(save)
		})
		if !game.MemoryAuto {
			kit.FoldMotion(ui.Column(c.Key("manual-memory")).FillWidth()).Children(func() {
				kit.Divider(c)
				kit.SettingRow(c, locales.MemoryLimit(), "", func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						hi := memory.TotalMiB
						if hi <= 0 {
							hi = math.MaxInt32
						}
						lo := min(512, hi)
						if p.memoryModel != game.MemoryMiB {
							p.memoryModel = game.MemoryMiB
							p.memoryValue = float64(game.MemoryMiB)
						}
						kit.Slider(c, &p.memoryValue, float64(lo), float64(hi)).Width(kit.W(160)).
							Label(locales.GameMemory()).Disabled(memory.TotalMiB == 0).OnChange(func() {
							game.MemoryMiB = max(lo, min(int(math.Round(p.memoryValue)), hi))
							p.memoryModel = game.MemoryMiB
							save()
						})
						c.Key("memoryMiB")
						p.memoryInput.input(c, &game.MemoryMiB, lo, hi, 88, locales.GameMemoryInMiB(), save, locales.MiB())
					})
				})
			})
		}
		kit.Divider(c)
		memoryStatus(c, game, memory)
		if memoryError != "" {
			kit.ErrorText(c, memoryError)
		}
		kit.Divider(c)
		kit.SettingRow(c, locales.ProcessPriority(), "", func() {
			processPriorityInput(c, &game.ProcessPriority, save)
		})
	})
}

func memoryStatus(c *ui.Context, game *config.Game, memory systeminfo.Memory) {
	allocated := game.MemoryMiB
	if game.MemoryAuto {
		allocated = memory.SuggestedMiB()
	}
	used := max(0, min(memory.UsedMiB, memory.TotalMiB))
	allocation := max(0, min(allocated, memory.TotalMiB-used))
	text := locales.Loading2()
	if memory.TotalMiB > 0 {
		text = fmt.Sprintf(locales.GiBUsed(), float64(used)/1024, float64(memory.TotalMiB)/1024)
	}
	kit.FormWideRow(c, locales.Memory(), text, func() {
		game, other := float64(0), float64(0)
		if memory.TotalMiB > 0 {
			game = float64(allocation) / float64(memory.TotalMiB)
			other = float64(used) / float64(memory.TotalMiB)
		}
		kit.MeterBar(c, game, other).Label(fmt.Sprintf(locales.MemoryUsedMiBGameLimitMiBTotalMiB(), used, allocated, memory.TotalMiB))
		kit.MeterLegend(c, fmt.Sprintf(locales.AllocatedToGameGiB(), float64(allocated)/1024), fmt.Sprintf(locales.SystemUsedGiB(), float64(used)/1024)).Wrap()
	})
}
