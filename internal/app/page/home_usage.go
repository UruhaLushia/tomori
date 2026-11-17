package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func homeUsage(c *ui.Context, s HomeUsage) {
	kit.SectionHeader(c, locales.ResourceUsage(), func() { kit.MeterLegend(c, locales.Game(), locales.Other()) })
	cpuValue, memValue := "—", "—"
	cpuTip, memTip := "", ""
	cpuGame, cpuOther, memGame, memOther := float64(0), float64(0), float64(0), float64(0)
	if s.Ready {
		cpuValue = fmt.Sprintf("%.0f / %.0f%%", s.CPUGame, s.CPUTotal)
		cpuGame, cpuOther = s.CPUGame/100, max(0, s.CPUTotal-s.CPUGame)/100
		cpuTip = fmt.Sprintf(locales.GameUsed(), s.CPUGame, s.CPUTotal)
	}
	if s.MemTotalMiB > 0 {
		memValue = fmt.Sprintf("%.1f / %.1f / %.0f GB", float64(s.MemGameMiB)/1024, float64(s.MemUsedMiB)/1024, float64(s.MemTotalMiB)/1024)
		memGame = float64(s.MemGameMiB) / float64(s.MemTotalMiB)
		memOther = float64(max(0, s.MemUsedMiB-s.MemGameMiB)) / float64(s.MemTotalMiB)
		memTip = fmt.Sprintf(locales.GameGbUsedGbTotalGb(), float64(s.MemGameMiB)/1024, float64(s.MemUsedMiB)/1024, float64(s.MemTotalMiB)/1024)
	}
	totalGB := float64(max(0, s.MemTotalMiB)) / 1024
	valueWidth := max(kit.W(80), kit.TextWidth(c, "100 / 100%", kit.Caption), kit.TextWidth(c, fmt.Sprintf("%.1f / %.1f / %.0f GB", totalGB, totalGB, totalGB), kit.Caption))
	homeUsageMeter(c, "CPU", cpuGame, cpuOther, cpuValue, cpuTip, valueWidth)
	homeUsageMeter(c, locales.MemoryLabel(), memGame, memOther, memValue, memTip, valueWidth)
	launcherCPU, launcherMemory := "CPU —", "—"
	if s.LauncherMiB > 0 {
		launcherCPU = fmt.Sprintf("CPU %.1f%%", max(0, s.CPULauncher))
		launcherMemory = fmt.Sprintf("%d MB", s.LauncherMiB)
	}
	launcherLabel := locales.Launcher()
	launcherLabelWidth := max(kit.W(40), kit.TextWidth(c, launcherLabel, kit.Caption)+kit.W(4))
	ui.Row(c).FillWidth().Height(kit.H(28, kit.Caption)).Margin(4, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		kit.Line(c, launcherLabel, kit.Caption).Width(launcherLabelWidth).Shrink(0)
		kit.Label(c, launcherCPU, kit.Caption).Grow(1).MinWidth(16).SingleLine().TextColor(kit.Text).ContentTransition(kit.NumberMotion)
		kit.Label(c, launcherMemory, kit.Caption).Width(kit.W(80)).Shrink(0).SingleLine().TextAlign(ui.End).TextColor(kit.Text).ContentTransition(kit.NumberMotion)
	})
}

func homeUsageMeter(c *ui.Context, label string, game, other float64, value, tooltip string, valueWidth float32) {
	ui.Column(c).FillWidth().Gap(4).Tooltip(tooltip).Children(func() {
		ui.Row(c).FillWidth().Height(kit.H(18, kit.Caption)).AlignItems(ui.Center).Children(func() {
			kit.Label(c, label, kit.Caption).Grow(1).MinWidth(0).SingleLine()
			kit.Label(c, value, kit.Caption).TextColor(kit.Text).Width(valueWidth).Shrink(0).SingleLine().TextAlign(ui.End).ContentTransition(kit.NumberMotion)
		})
		kit.MeterBar(c, game, other)
	})
}
