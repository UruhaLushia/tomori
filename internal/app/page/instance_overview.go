package page

import (
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
	icon  *ui.SVG
}{
	{"", locales.Overview, kit.Home}, {"worlds", locales.Worlds, kit.Globe}, {string(resource.Mod), locales.Mods, kit.Puzzle},
	{string(resource.ResourcePack), locales.ResourcePacks, kit.Layers}, {string(resource.ShaderPack), locales.Shaders, kit.Sun}, {"screenshots", locales.Screenshots, kit.Image},
	{"servers", locales.Servers, kit.Server},
}

func (p *Instances) changeTab(tab string, actions InstancesActions) {
	if p.tab == tab {
		return
	}
	p.tab, p.files = tab, ui.ListState{}
	for i, item := range instanceTabs {
		if item.id == tab {
			p.tabForward = i > p.tabIndex
			p.tabIndex = i
			p.tabStrip.Reveal(i)
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
	tabs := make([]kit.ScrollTab, 0, len(instanceTabs))
	for _, tab := range instanceTabs {
		count := 0
		switch tab.id {
		case "":
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
		id := tab.id
		tabs = append(tabs, kit.ScrollTab{Label: tab.label(), Icon: tab.icon, Selected: p.tab == id, Count: count, OnClick: func() { p.changeTab(id, actions) }})
	}
	kit.ScrollTabs(c, &p.tabStrip, tabs)
}

func (p *Instances) overview(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	p.libraryOverview(c, v, data, actions)
}
