package page

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"strings"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

func MatchInstance(v minecraft.Instance, query string) bool {
	return strings.Contains(strings.ToLower(v.Name+" "+v.GameVersion+" "+v.Loader+" "+v.SourceName), strings.ToLower(strings.TrimSpace(query)))
}

type LibraryStatistics struct {
	Mods    int
	Known   bool
	Seconds int64
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
				ui.Icon(c, item.icon).FontSize(kit.IconPx(16)).TextColor(color)
			})
		}
	})
}

func (p *Instances) libraryOverview(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Wrap().Gap(kit.ColGap(c)).Padding(0, 0, kit.Pad(c), 0).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(3).Basis(kit.W(240)).Shrink(1).MinWidth(0).Gap(24).Children(func() {
				if data.Content.Error != "" {
					kit.ErrorText(c, data.Content.Error)
				}
				ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
					for _, item := range []struct {
						id    string
						label func() string
						count int
					}{
						{string(resource.Mod), locales.Mods, data.Content.Summary.Resources[resource.Mod].Total},
						{string(resource.ResourcePack), locales.ResourcePacks, data.Content.Summary.Resources[resource.ResourcePack].Total},
						{string(resource.ShaderPack), locales.Shaders, data.Content.Summary.Resources[resource.ShaderPack].Total},
						{"screenshots", locales.Screenshots, len(data.Content.Summary.Screenshots)},
					} {
						c.Key(item.id)
						value := fmt.Sprint(item.count)
						if data.Content.Loading {
							value = "…"
						}
						kit.StatBlock(c, item.label(), value, func() { p.changeTab(item.id, actions) })
					}
				})
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.SectionHeader(c, locales.RecentWorlds(), func() { kit.TextButton(c, locales.All2(), func() { p.changeTab("worlds", actions) }) })
					p.recentWorlds(c, data, actions)
				})
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.SectionHeader(c, locales.RecentScreenshots(), func() { kit.TextButton(c, locales.All2(), func() { p.changeTab("screenshots", actions) }) })
					p.recentScreenshots(c, data, actions)
				})
			})
			ui.Column(c).Grow(2).Basis(kit.W(220)).Shrink(1).MinWidth(0).MaxWidth(kit.W(340)).Gap(12).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					kit.SectionHeader(c, locales.Information(), nil)
					kit.Property(c, locales.GameVersion(), v.GameVersion)
					if v.Loader != "" {
						kit.Property(c, locales.Loader(), v.Loader)
					}
					kit.Property(c, locales.Source3(), v.SourceName)
					if v.PackFormat != "" {
						kit.Property(c, locales.Modpack(), kit.MetaLine(v.PackFormat, v.PackVersion))
					}
					kit.Property(c, locales.Directory(), data.Content.Root)
				})
				if v.Description != "" {
					kit.Label(c, v.Description, kit.Caption).Wrap().MaxLines(3).Ellipsis("…")
				}
				ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {

					kit.Ghost(c, locales.CopyLaunchLink(), func() {
						if actions.CopyLink != nil {
							actions.CopyLink(v.Key())
						}
					})
				})
			})
		})
	})
}

func (p *Instances) libraryListBody(c *ui.Context, v minecraft.Instance, data InstancesData) {
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
		if data.Activity != nil && data.Activity(v.Key()).Active {
			kit.StatusDot(c, kit.Green)
		}
	})
}
