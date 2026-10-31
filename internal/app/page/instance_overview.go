package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type InstanceContentData struct {
	Key, Root, Error string
	Loading          bool
	Summary          instancecontent.Summary
	Thumbnails       map[string]*ui.Bitmap
	Worlds           map[string]instancecontent.World
	PreviewPath      string
	Preview          *ui.Bitmap
	PreviewLoading   bool
	PreviewError     string
}

var instanceTabs = [...]struct {
	id    string
	label func() string
}{
	{"", locales.Overview}, {"worlds", locales.Worlds}, {string(resource.Mod), locales.Mods},
	{string(resource.ResourcePack), locales.ResourcePacks}, {string(resource.ShaderPack), locales.Shaders}, {"screenshots", locales.Screenshots},
	{"servers", locales.Servers},
}

type instanceTabDrag struct {
	down, moved bool
	tab         int
	x, y, from  float32
}

func (p *Instances) tabAt(c *ui.Context, x, y float32) int {
	strip := p.tabScroll.Bounds(c)
	point := ui.Point{X: strip.X + x, Y: strip.Y + y}
	if !strip.Contains(point.X, point.Y) {
		return -1
	}
	for i, tab := range p.tabHandles {
		if tab.Bounds(c).Contains(point.X, point.Y) {
			return i
		}
	}
	return -1
}

func (p *Instances) tabInput(c *ui.Context, ev ui.InputEvent, actions InstancesActions) bool {
	drag := &p.tabDrag
	switch ev.Kind {
	case ui.InputScroll:
		if ev.DX != 0 || ev.DY == 0 || ev.Mods != 0 {
			return false
		}
		before := p.tabScroll.X
		p.tabScroll.X = max(0, min(p.tabScroll.MaxX, before+ev.DY))
		return p.tabScroll.X != before
	case ui.InputPointerDown:
		if ev.Button != 0 || p.tabScroll.MaxX <= 0 {
			return false
		}
		*drag = instanceTabDrag{down: true, tab: p.tabAt(c, ev.X, ev.Y), x: ev.X, y: ev.Y, from: p.tabScroll.X}
		return true
	case ui.InputPointerMove:
		if !drag.down {
			return false
		}
		dx, dy := ev.X-drag.x, ev.Y-drag.y
		if dx*dx+dy*dy > 36 {
			drag.moved = true
		}
		if drag.moved {
			if c.Direction() == ui.RightToLeft {
				dx = -dx
			}
			p.tabScroll.X = max(0, min(p.tabScroll.MaxX, drag.from-dx))
		}
		return true
	case ui.InputPointerUp:
		if ev.Button != 0 || !drag.down {
			return false
		}
		chosen := drag.tab
		click := !drag.moved && chosen >= 0 && p.tabScroll.FocusWithin(c) && p.tabAt(c, ev.X, ev.Y) == chosen
		*drag = instanceTabDrag{}
		if click {
			p.changeTab(instanceTabs[chosen].id, actions)
		}
		return true
	}
	return false
}

func (p *Instances) changeTab(tab string, actions InstancesActions) {
	if p.tab == tab {
		return
	}
	p.tab, p.files = tab, ui.ListState{}
	p.tabReveal = true
	for i, item := range instanceTabs {
		if item.id == tab {
			p.tabForward = i > p.tabIndex
			p.tabIndex = i
			break
		}
	}
	if tab == "servers" && actions.ServerActions.Refresh != nil {
		actions.ServerActions.Refresh()
	} else if kind := resource.Kind(tab); kind.Valid() {
		actions.Content(kind)
	} else {
		actions.RefreshContent()
	}
}

func (p *Instances) contentTabs(c *ui.Context, data InstancesData, actions InstancesActions) {
	compact := kit.Compact(c)
	ui.ScrollHorizontal(c).FillWidth().MinWidth(0).Height(kit.TabH()).Shrink(0).Gap(4).TrackScroll(&p.tabScroll).Scrollbars(false).HandleInput(func(ev ui.InputEvent) bool {
		return p.tabInput(c, ev, actions)
	}).Children(func() {
		for i, tab := range instanceTabs {
			c.Key(tab.id)
			el := kit.NavTab(c, tab.label(), p.tab == tab.id, func() { p.changeTab(tab.id, actions) }).Shrink(0).Bind(&p.tabHandles[i])
			if p.tabReveal && p.tab == tab.id {
				el.ScrollIntoView()
			}
			if !compact {
				count := 0
				switch tab.id {
				case "worlds":
					count = len(data.Content.Summary.Worlds)
				case "screenshots":
					count = len(data.Content.Summary.Screenshots)
				case "servers":
					if data.Servers != nil {
						count = len(data.Servers.List.Entries)
					}
				default:
					count = data.Content.Summary.Resources[resource.Kind(tab.id)].Total
				}
				if count > 0 {
					el.Label(fmt.Sprintf("%s %d", tab.label(), count)).Gap(4).Children(func() { kit.Label(c, fmt.Sprint(count), kit.Caption) })
				}
			}
		}
	})
	p.tabReveal = false
}

func (p *Instances) overview(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	p.libraryOverview(c, v, data, actions)
}

func (p *Instances) recentWorlds(c *ui.Context, data InstancesData, actions InstancesActions) {
	files := data.Content.Summary.Worlds
	if len(files) > 3 {
		files = files[:3]
	}
	if len(files) == 0 {
		kit.Label(c, locales.NoWorlds(), kit.Caption)
		return
	}
	for _, file := range files {
		c.Key(file.Path)
		world := data.Content.Worlds[file.Path]
		name, meta := worldSummary(file, world)
		row := kit.RowButton(c, "", false, func() { p.worldPath = file.Path }).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12)
		if row.InView() && actions.LoadWorld != nil {
			actions.LoadWorld(file)
		}
		kit.PresenceMotion(row).Children(func() {
			kit.IconBadge(c, kit.Folder, 32)
			ui.Column(c).Grow(1).MinWidth(0).Gap(0).Children(func() { kit.Line(c, name, kit.Strong); kit.Line(c, meta, kit.Caption) })
			kit.Line(c, file.Modified.Format("01-02 15:04"), kit.Caption).Shrink(0)
		})
	}
}

func (p *Instances) recentScreenshots(c *ui.Context, data InstancesData, actions InstancesActions) {
	files := data.Content.Summary.Screenshots
	if len(files) > 3 {
		files = files[:3]
	}
	if len(files) == 0 {
		kit.Label(c, locales.NoScreenshots(), kit.Caption)
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(kit.S8).Children(func() {
		for _, file := range files {
			c.Key(file.Path)
			thumb := ui.ButtonBase(c).Grow(1).Basis(160).MinWidth(160).AspectRatio(16.0 / 9.0).Radius(kit.RadiusM).Clip().OnClick(func() { actions.Preview(file.Path) }).Label(locales.PreviewScreenshot() + file.Name)
			kit.PresenceMotion(thumb)
			thumb.Children(func() { mediaImage(c, file, data, actions, false) })
		}
	})
}

func worldSummary(file instancecontent.File, world instancecontent.World) (string, string) {
	name := file.Name
	if world.Name != "" {
		name = world.Name
	}
	parts := []string{}
	if world.Version != "" {
		parts = append(parts, world.Version)
	}
	if world.Valid {
		parts = append(parts, worldMode(world))
	}
	parts = append(parts, file.Modified.Format("2006-01-02"))
	return name, kit.MetaLine(parts...)
}
