package page

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type Statistics struct {
	selectedDay int
	chartMode   int
	ready       bool
}

type StatisticsData struct {
	Statistics HomeStatistics
	Icons      map[string]*ui.Bitmap
}

func (p *Statistics) View(c *ui.Context, data StatisticsData) {
	if !p.ready {
		p.selectedDay, p.ready = 6, true
	}
	ui.Column(c).FillWidth().Shrink(0).Gap(16).Children(func() {
		kit.Line(c, locales.StatisticsSubtitle(), kit.Caption).TextColor(kit.Muted)
		p.overview(c, data)
	})
}

func (p *Statistics) overview(c *ui.Context, data StatisticsData) {
	s := data.Statistics
	days, launchDays, launches := playSummary(s)
	total := float64(0)
	activeDays := 0
	for _, value := range days {
		total += value
		if value > 0 {
			activeDays++
		}
	}
	average := float64(0)
	if launches > 0 {
		average = total / float64(launches)
	}
	ui.Row(c).FillWidth().Wrap().Gap(12).Children(func() {
		statisticsMetric(c, locales.PlayTime(), playDuration(total), locales.Last7Days())
		statisticsMetric(c, locales.LaunchCount(), fmt.Sprintf("%d", launches), locales.Last7Days())
		statisticsMetric(c, locales.ActiveDays(), fmt.Sprintf("%d", activeDays), locales.Last7Days())
		statisticsMetric(c, locales.AverageSession(), playDuration(average), locales.Last7Days())
	})
	ui.Row(c).FillWidth().Wrap().Gap(16).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(2).Basis(480).MinWidth(0).Shrink(1).Gap(16).Children(func() {
			statisticsPanel(c, locales.TimeTrend(), func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
					subtitle := locales.DailyPlayTime()
					if p.chartMode == 1 {
						subtitle = locales.LaunchCount()
					}
					kit.Line(c, subtitle, kit.Caption).Grow(1)
					kit.Segmented(c, &p.chartMode, locales.PlayTime(), locales.LaunchCount())
				})
				values := days
				if p.chartMode == 1 {
					for i, value := range launchDays {
						values[i] = float64(value)
					}
				}
				statisticsBars(c, values, p.chartMode, 148, p.selectedDay, func(day int) { p.selectedDay = day })
			})
			statisticsPanel(c, locales.DayDetail(), func() {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					kit.Label(c, playDuration(days[p.selectedDay]), kit.Strong)
					ui.Box(c).Grow(1).MinWidth(0)
					kit.Label(c, fmt.Sprintf(locales.Launches(), launchDays[p.selectedDay]), kit.Caption)
				})
				ui.Column(c).FillWidth().Gap(6).Margin(0, 8).Children(func() {
					for _, pack := range s.Packs {
						seconds := pack.Days[p.selectedDay]
						if seconds <= 0 && pack.LaunchesByDay[p.selectedDay] == 0 {
							continue
						}
						ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
							kit.InstanceIcon(c, data.Icons[pack.Key], 20)
							kit.Label(c, pack.Name, kit.Caption).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
							kit.Label(c, playDuration(seconds), kit.Caption).Shrink(0).TextColor(kit.Muted)
						})
					}
				})
			})
		})
		ui.Column(c).Grow(1).Basis(280).MinWidth(240).Shrink(1).Gap(16).Children(func() {
			statisticsPanel(c, locales.GameDistribution(), func() {
				if len(s.Packs) == 0 {
					kit.EmptyState(c, kit.Cube, locales.NoPlayHistory(), "", nil)
					return
				}
				ui.Column(c).FillWidth().Gap(12).Children(func() {
					for _, pack := range s.Packs {
						fraction := float64(0)
						if total > 0 {
							fraction = pack.Total / total
						}
						ui.Column(c).FillWidth().Gap(4).Children(func() {
							ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
								kit.InstanceIcon(c, data.Icons[pack.Key], 28)
								ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
									kit.Label(c, pack.Name, kit.Strong).SingleLine().Ellipsis("…")
									kit.Label(c, kit.MetaLine(playDuration(pack.Total), fmt.Sprintf(locales.Launches(), pack.Launches)), kit.Caption).SingleLine().Ellipsis("…")
								})
								kit.Label(c, fmt.Sprintf("%.0f%%", fraction*100), kit.Caption).TextColor(kit.AccentText).Shrink(0)
							})
							kit.MeterBar(c, fraction, 0)
						})
					}
				})
			})
			statisticsPanel(c, locales.ActivityPattern(), func() { statisticsActivity(c, s.Activity) })
		})
	})
}

func statisticsMetric(c *ui.Context, label, value, hint string) {
	ui.Column(c).Grow(1).Basis(180).MinWidth(160).Padding(16).Gap(6).Radius(kit.RadiusL).Background(kit.Surface).Border(1, kit.LineSoft).Children(func() {
		kit.Label(c, label, kit.Caption).TextColor(kit.Muted)
		kit.Label(c, value, kit.Heading).SingleLine().Shrink(0).ContentTransition(kit.NumberMotion)
		kit.Label(c, hint, kit.Caption).TextColor(kit.Muted).SingleLine()
	})
}

func statisticsPanel(c *ui.Context, title string, build func()) {
	ui.Column(c).FillWidth().Padding(16).Gap(8).Radius(kit.RadiusL).Background(kit.Surface).Border(1, kit.LineSoft).Children(func() {
		kit.Label(c, title, kit.Strong)
		build()
	})
}

func statisticsBars(c *ui.Context, days [7]float64, mode int, height float32, selected int, onClick func(int)) {
	peak := float64(0)
	for _, value := range days {
		peak = max(peak, value)
	}
	unit, suffix := float64(1), ""
	if mode == 0 {
		unit, suffix = 3600, "h"
	}
	step := max(1, math.Ceil(peak/unit/3))
	peak = step * unit * 3
	var labels [4]string
	axisWidth := float32(0)
	for i := range labels {
		labels[i] = fmt.Sprintf("%.0f%s", float64(3-i)*step, suffix)
		axisWidth = max(axisWidth, kit.TextWidth(c, labels[i], kit.Caption))
	}
	today := time.Now()
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Padding(8, 8, 8, 0).Children(func() {
		ui.Box(c).Width(axisWidth).Height(height).Shrink(0).Children(func() {
			for i, label := range labels {
				kit.Label(c, label, kit.Caption).Absolute().Right(0).Top(float32(i)*height/3 - kit.LinePx(kit.Caption)/2).SingleLine()
			}
		})
		ui.Box(c).Grow(1).MinWidth(0).Children(func() {
			for i := range labels {
				ui.Box(c).Absolute().Left(0).Right(0).Top(float32(i) * height / float32(len(labels)-1)).Height(1).Background(kit.LineSoft).PassThrough()
			}
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				for i, value := range days {
					c.Key(i)
					button := ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Padding(0).Radius(kit.RadiusS).OnClick(func() { onClick(i) })
					button.Children(func() {
						ui.Column(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
							tooltip := playDuration(value)
							if mode == 1 {
								tooltip = fmt.Sprintf(locales.Launches(), int(value))
							}
							kit.ChartBar(c, value, peak, height, i == selected).Tooltip(tooltip)
							day := today.AddDate(0, 0, i-6).Weekday()
							label := []func() string{locales.Sun, locales.Mon, locales.Tue, locales.Wed, locales.Thu, locales.Fri, locales.Sat}[day]
							color := kit.Muted
							if i == selected {
								color = kit.AccentText
							}
							if i == 6 {
								label = locales.TodayCompact
							}
							kit.Label(c, label(), kit.Caption).SingleLine().Shrink(0).TextColor(color)
						})
					})
				}
			})
		})
	})
}

func statisticsActivity(c *ui.Context, activity [24]float64) {
	var bins [12]float64
	peak := float64(0)
	for hour, value := range activity {
		bins[hour/2] += value
	}
	for _, value := range bins {
		peak = max(peak, value)
	}
	ui.Row(c).FillWidth().Gap(6).Padding(8, 8, 0, 8).Children(func() {
		for i, value := range bins {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
				kit.ChartBar(c, value, peak, 72, false).Tooltip(fmt.Sprintf("%02d:00", i*2) + " · " + playDuration(value))
				kit.Label(c, fmt.Sprintf("%02d", i*2), kit.Caption).SingleLine().Shrink(0).TextColor(kit.Muted)
			})
		}
	})
}
