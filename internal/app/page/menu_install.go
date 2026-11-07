package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Instances) installMenu(c *ui.Context, busy bool, newInstall, importPack, browsePacks func()) {
	c.Key("install-menu")
	menu := kit.SelectBase(c, &p.installAction)
	var trigger ui.Element
	if p.inline {
		trigger = kit.SelectTrigger(c, menu.Trigger.Disabled(busy), menu.Open()).Size(32, 32).Padding(0).Center().Label(locales.NewInstance())
		trigger.Children(func() { ui.Icon(c, kit.Plus).FontSize(16) })
	} else {
		trigger = kit.PrimaryMenuButton(c, menu.Trigger.Disabled(busy), locales.New2(), menu.Open())
	}
	trigger.OnChange(func() {
		action := p.installAction
		p.installAction = ""
		switch action {
		case "new":
			newInstall()
		case "import":
			importPack()
		case "browse":
			if browsePacks != nil {
				browsePacks()
			}
		}
	})
	menu.Popup(func(panel ui.Element) {
		kit.PopupPanel(c, panel, 168, menu.Trigger).Role(ui.RoleMenu)
		for _, option := range []struct{ value, label string }{{"new", locales.NewInstance()}, {"import", locales.ImportModpack()}, {"browse", locales.BrowseOnlineModpacks()}} {
			c.Key(option.value)
			kit.MenuItem(c, menu.Item(option.value).Disabled(busy), option.label)
		}
	})
}
