package page

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Accounts) clearExternal() {
	p.externalURL, p.externalName, p.externalPassword, p.externalProfile, p.externalID = "", "", "", "", ""
}

func (p *Accounts) CloseExternal() {
	if p.addKind == "yggdrasil" {
		p.addOpen, p.waiting = false, false
		p.clearExternal()
	}
}

func (p *Accounts) openExternal(identity account.Account) {
	p.clearExternal()
	p.addKind, p.addOpen, p.waiting = "yggdrasil", true, false
	p.externalURL, p.externalName, p.externalID = identity.ServerURL, identity.LoginName, identity.ID
}

func (p *Accounts) externalDialog(c *ui.Context, data AccountData, actions AccountActions) {
	locked := data.Locked || data.Busy
	if data.ExternalServer.URL == "" {
		kit.Label(c, locales.AuthenticationServer(), kit.Body).TextColor(kit.Muted)
		c.Key("external-server")
		kit.Input(c, &p.externalURL).FillWidth().Height(kit.ControlH()).Label(locales.AuthenticationServerAddress()).Placeholder("https://").Disabled(locked || p.externalID != "").OnSubmit(func() { actions.ExternalInspect(p.externalURL) })
		if strings.HasPrefix(strings.TrimSpace(p.externalURL), "http:") {
			kit.ErrorText(c, locales.HttpSendsYourAccountAndPasswordWithoutEncryption()).Wrap()
		}
		kit.DialogFooter(c, func() {
			kit.Primary(c, locales.Connect(), func() { actions.ExternalInspect(p.externalURL) }).Disabled(locked || strings.TrimSpace(p.externalURL) == "")
		})
	} else {
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			kit.Label(c, data.ExternalServer.Name, kit.Body).MaxLines(2).Ellipsis("…").Wrap().Tooltip(data.ExternalServer.Name)
			kit.Label(c, data.ExternalServer.URL, kit.Body).TextColor(kit.Muted).MaxLines(2).Ellipsis("…").Wrap().Tooltip(data.ExternalServer.URL)
			if strings.HasPrefix(data.ExternalServer.URL, "http:") {
				kit.ErrorText(c, locales.HttpSendsYourAccountAndPasswordWithoutEncryption()).Wrap()
			}
		})
		if len(data.ExternalProfiles) > 0 {
			kit.Label(c, locales.ChooseProfile(), kit.Body).TextColor(kit.Muted)
			c.Key("external-profile")
			profile := kit.SelectBase(c, &p.externalProfile)
			label := locales.ChooseAProfile()
			for _, item := range data.ExternalProfiles {
				if item.ID == p.externalProfile {
					label = item.Name
					break
				}
			}
			kit.MenuButton(c, profile.Trigger.Disabled(locked), label, profile.Open()).FillWidth().Label(locales.ChooseProfile())
			profile.Popup(func(panel ui.Element) {
				kit.PopupPanel(c, panel, 280, profile.Trigger)
				ui.Scroll(c).Scrollbars(false).FillWidth().MaxHeight(240).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						for _, item := range data.ExternalProfiles {
							c.Key(item.ID)
							kit.MenuItem(c, profile.Item(item.ID), item.Name)
						}
					})
				})
			})
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.Back(), func() { actions.ExternalBack(); p.externalProfile = "" }).Disabled(locked)
				kit.Primary(c, locales.Add(), func() { actions.ExternalSelect(p.externalProfile); p.waiting = true }).Disabled(locked || p.externalProfile == "")
			})
		} else {
			login := func() {
				if locked || strings.TrimSpace(p.externalName) == "" || p.externalPassword == "" {
					return
				}
				password := p.externalPassword
				p.externalPassword = ""
				actions.ExternalLogin(p.externalName, password)
				p.waiting = true
			}
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				c.Key("external-login")
				kit.Input(c, &p.externalName).FillWidth().Height(kit.ControlH()).Label(locales.LoginAccount()).Placeholder(locales.LoginAccount()).Disabled(locked || p.externalID != "")
				c.Key("external-password")
				kit.Input(c, &p.externalPassword).Password().FillWidth().Height(kit.ControlH()).Label(locales.Password()).Placeholder(locales.Password()).Disabled(locked).OnSubmit(login)
			})
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.Back(), actions.ExternalBack).Disabled(locked)
				kit.Primary(c, locales.SignIn(), login).Disabled(locked || strings.TrimSpace(p.externalName) == "" || p.externalPassword == "")
			})
		}
	}
	if data.Busy {
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() { ui.Spinner(c); kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).Wrap() })
	}
}
