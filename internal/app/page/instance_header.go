package page

import (
	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Instances) instanceHeader(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	if actions.LoadIcon != nil {
		actions.LoadIcon(v)
	}
	ui.Column(c).FillWidth().Shrink(0).Gap(12).Children(func() {
		if !p.inline {
			kit.BackBar(c, locales.Library(), func() { p.leaveDetail(actions) }, nil)
		}
		ui.Column(c).FillWidth().Shrink(0).Gap(kit.Gap(c)).Children(func() {
			kit.IdentityHeader(c, kit.Identity{Icon: data.Icons[v.Key()], Name: v.Name, Meta: kit.MetaLine(v.GameVersion, v.Loader, v.SourceName)}, nil, func() {
				kit.IconButton(c, kit.Folder, false, func() { actions.Open(instanceRoot(v, data)) }).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
				if kit.Compact(c) {
					p.instanceIconMenu(c, v, data, actions)
				} else {
					p.instanceMenu(c, v, data, actions)
				}
			})
			p.contentTabs(c, data, actions)
		})
	})
}
