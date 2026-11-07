package page

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func MatchInstance(v minecraft.Instance, query string) bool {
	return strings.Contains(strings.ToLower(v.Name+" "+v.GameVersion+" "+v.Loader+" "+v.SourceName), strings.ToLower(strings.TrimSpace(query)))
}

type LibraryStatistics struct {
	Mods       int
	Known      bool
	Seconds    int64
	LastPlayed time.Time
}

func (p *Instances) viewSwitcher(c *ui.Context, data InstancesData, actions InstancesActions) {
	parts := ui.SegmentedBase(c, &p.viewMode, 2)
	parts.Track.Height(kit.ControlH()).Shrink(0).Padding(4).Gap(4).Radius(kit.RadiusM).Background(kit.InsetColor).
		OnChange(func() {
			p.reorder.cancel()
			p.grid = ui.GridState{}
			p.inline, p.detail = p.viewMode == 0, false
			if data.ViewMode != nil {
				*data.ViewMode = p.viewMode
			}
			if actions.ViewChanged != nil {
				actions.ViewChanged()
			}
		}).Children(func() {
		for i, item := range []struct {
			name func() string
			icon *ui.SVG
		}{{locales.List, kit.List}, {locales.Grid, kit.Grid}} {
			segment := parts.Segment(i).Height(kit.H(24, kit.Body)).Padding(0, 8).Radius(kit.RadiusS).Center().Label(item.name())
			background, color := ui.Transparent, kit.Muted
			if i == p.viewMode {
				background, color = kit.AccentSoft, kit.AccentText
			} else if segment.Hovered() {
				background = kit.Hover
			}
			kit.ControlMotion(segment).Background(segment.AnimateColor("bg", background, kit.MotionFast)).Children(func() {
				ui.Icon(c, item.icon).FontSize(16).TextColor(color)
			})
		}
	})
}

func (p *Instances) libraryOverview(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(12).Padding(0, 0, kit.Pad(c), 0).Children(func() {
			if data.Content.Error != "" {
				kit.ErrorText(c, data.Content.Error)
			}
			overviewInfo(c, v, data)
			ui.Row(c).FillWidth().Wrap().Gap(12).AlignItems(ui.Stretch).Children(func() {
				p.overviewWorlds(c, data, actions).Grow(3).Basis(kit.W(300))
				if len(data.Content.Summary.Screenshots) > 0 {
					p.overviewScreenshot(c, data, actions)
				}
			})
		})
	})
}

func overviewCard(c *ui.Context, key, title string, watermark *ui.SVG, trailing func(), body func()) ui.Element {
	card := ui.Column(c.Key(key)).MinWidth(0).Padding(12, 16, 16, 16).Gap(12).Radius(kit.RadiusL).Background(kit.Surface).Border(1, kit.LineSoft).Clip()
	if watermark != nil {
		color := kit.Accent.Alpha(.07)
		card.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Icon(watermark, ui.Rect{X: r.X + r.W - 92, Y: r.Y + r.H - 92, W: 84, H: 84}, color)
		})
	}
	return kit.LayoutMotion(card).Children(func() {
		ui.Row(c).FillWidth().Height(kit.TabH()).Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Line(c, title, kit.Strong).Grow(1)
			if trailing != nil {
				trailing()
			}
		})
		body()
	})
}

func overviewFact(c *ui.Context, icon *ui.SVG, label, value, detail string) {
	ui.Row(c).Grow(1).Basis(kit.W(180)).MinWidth(0).Gap(12).AlignItems(ui.Center).Children(func() {
		kit.IconBadge(c, icon, 40, kit.AccentText)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			kit.Line(c, label, kit.Caption)
			kit.Line(c, value, kit.Strong)
			if detail != "" {
				kit.Line(c, detail, kit.Caption)
			}
		})
	})
}

func overviewInfo(c *ui.Context, v minecraft.Instance, data InstancesData) {
	overviewCard(c, "info", locales.Information(), nil, nil, func() {
		ui.Row(c).FillWidth().Wrap().Gap(16).Children(func() {
			loader := v.Loader
			if loader == "" {
				loader = locales.Vanilla()
			}
			overviewFact(c, kit.Cube, locales.GameVersion(), v.GameVersion, loader)
			stat := data.Statistics[v.Key()]
			played := locales.NotPlayedYet()
			if stat.Seconds > 0 || !stat.LastPlayed.IsZero() {
				played = fmt.Sprintf("%.1f %s", float64(stat.Seconds)/3600, locales.Hours())
			}
			overviewFact(c, kit.Clock, locales.PlayTime(), played, "")
			if when := relativePlayTime(stat.LastPlayed); when != "" {
				overviewFact(c, kit.Play, locales.LastPlayed(), when, "")
			}
			if v.PackFormat != "" {
				overviewFact(c, kit.Layers, locales.Modpack(), v.PackFormat, v.PackVersion)
			}
			overviewFact(c, kit.Folder, locales.Source3(), v.SourceName, kit.MiddleEllipsis(instanceRoot(v, data), 32))
		})
	}).FillWidth()
}

func (p *Instances) overviewWorlds(c *ui.Context, data InstancesData, actions InstancesActions) ui.Element {
	var watermark *ui.SVG
	if len(data.Content.Summary.Worlds) > 0 {
		watermark = kit.Globe
	}
	return overviewCard(c, "worlds", locales.RecentWorlds(), watermark, func() {
		kit.TextButton(c, locales.All2(), func() { p.changeTab("worlds", actions) }).Margin(0, -8, 0, 0)
	}, func() {
		files := data.Content.Summary.Worlds
		if len(files) == 0 {
			ui.Row(c).FillWidth().Height(kit.RowDoubleH()).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Line(c, locales.NoWorlds(), kit.Body).Grow(1).TextColor(kit.Muted)
				p.worldImportMenu(c, data, actions)
			})
			return
		}
		ui.Column(c).FillWidth().Margin(-4, -8, -8, -8).Children(func() {
			for _, file := range files[:min(3, len(files))] {
				c.Key(file.Path)
				world := data.Content.Worlds[file.Path]
				name := file.Name
				if world.Name != "" {
					name = world.Name
				}
				meta := world.Version
				if world.Valid {
					meta = kit.MetaLine(meta, worldMode(world))
				}
				when := file.Modified
				if !world.LastPlayed.IsZero() {
					when = world.LastPlayed
				}
				row := kit.RowButton(c, name, false, func() { p.worldPath = file.Path }).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
				if row.InView() && actions.LoadWorld != nil {
					actions.LoadWorld(file)
				}
				row.Children(func() {
					ui.Box(c).Size(32, 32).Shrink(0).Radius(kit.RadiusS).Clip().Children(func() { mediaImage(c, file, data, actions, true) })
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						kit.Line(c, name, kit.Strong)
						kit.Line(c, meta, kit.Caption)
					})
					kit.Line(c, relativePlayTime(when), kit.Caption).Shrink(0)
				})
			}
		})
	})
}

func (p *Instances) overviewScreenshot(c *ui.Context, data InstancesData, actions InstancesActions) {
	files := data.Content.Summary.Screenshots
	card := ui.ButtonBase(c.Key("latest-screenshot")).Grow(2).Basis(kit.W(220)).MinWidth(0).MinHeight(150).Radius(kit.RadiusL).Clip().Label(locales.RecentScreenshots()).OnClick(func() { p.changeTab("screenshots", actions) })
	kit.CardMotion(card).Children(func() {
		mediaImage(c, files[0], data, actions, false)
		ui.Row(c).Absolute().Left(12).Top(12).Height(kit.H(24, kit.Caption)).Padding(0, 8).Gap(4).Radius(kit.RadiusS).Background(kit.Surface).AlignItems(ui.Center).PassThrough().Children(func() {
			kit.Label(c, locales.Screenshots(), kit.Caption).TextColor(kit.Text).SingleLine()
			kit.Label(c, fmt.Sprint(len(files)), kit.Caption).SingleLine()
		})
	})
}

func (p *Instances) libraryListBody(c *ui.Context, v minecraft.Instance, data InstancesData, actions *InstancesActions, hovered bool) {
	ui.Row(c).Fill().Gap(12).AlignItems(ui.Center).Children(func() {
		kit.InstanceIcon(c, data.Icons[v.Key()], 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(0).Children(func() {
			color := kit.Text
			if v.Key() == p.selected {
				color = kit.AccentText
			}
			kit.Line(c, v.Name, kit.Strong).TextColor(color)
			kit.Line(c, kit.MetaLine(v.GameVersion, v.Loader), kit.Caption)
		})
		activity := instanceActivity(v, data)
		switch {
		case actions != nil && hovered && (activity.Running || !activity.Active):
			p.quickLaunch(c, v, data, *actions)
		case activity.Active && !activity.Running:
			c.Key("launch-spinner")
			ui.Spinner(c).Size(14, 14).TextColor(kit.Accent)
		case activity.Running:
			kit.StatusDot(c, kit.Green)
		}
	})
}

func (p *Instances) quickLaunch(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) ui.Element {
	key := v.Key()
	activity := instanceActivity(v, data)
	busy := data.InstanceBusy
	if data.InUse != nil {
		busy = data.InUse(key)
	}
	label, disabled := locales.LaunchGame(), data.Busy || busy || !v.CanLaunch()
	run := func() {
		if actions.Launch != nil {
			actions.Launch(key)
		}
	}
	if activity.Running {
		label, disabled = locales.StopGame(), activity.Canceling
		run = func() {
			if actions.Stop != nil {
				actions.Stop(key)
			}
		}
	}
	c.Key("quick-launch")
	button := kit.QuickAction(c, activity.Running, label, run).Disabled(disabled)
	handle := ui.Local(button, "quick-launch", func() ui.Handle { return ui.Handle{} })
	button.Bind(handle)
	p.reorder.passthrough = append(p.reorder.passthrough, *handle)
	return button
}
