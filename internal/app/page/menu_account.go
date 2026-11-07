package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Accounts) addMenu(c *ui.Context, data AccountData, actions AccountActions) {
	c.Key("add-account-menu")
	menu := kit.SelectBase(c, &p.addAction)
	kit.MenuButton(c, menu.Trigger.Disabled(data.Locked || data.Busy), locales.AddAccount(), menu.Open()).
		OnChange(func() {
			kind := p.addAction
			p.addAction = ""
			if kind != "microsoft" && kind != "offline" && kind != "yggdrasil" {
				return
			}
			if actions.ClearError != nil {
				actions.ClearError()
			}
			p.addKind, p.addOpen = kind, true
			if kind == "yggdrasil" {
				p.clearExternal()
				actions.ExternalBegin("")
			}
			if kind == "microsoft" {
				actions.Login()
				p.waiting = true
			}
		})
	menu.Popup(func(panel ui.Element) {
		kit.PopupPanel(c, panel, 168, menu.Trigger).Role(ui.RoleMenu)
		for _, option := range []struct{ value, label string }{{"microsoft", locales.MicrosoftAccount()}, {"offline", locales.OfflineAccount()}, {"yggdrasil", locales.ExternalLogin()}} {
			c.Key(option.value)
			kit.MenuItem(c, menu.Item(option.value).Disabled(data.Locked || data.Busy), option.label)
		}
	})
}
