package page

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Accounts) addDialog(c *ui.Context, data AccountData, actions AccountActions) {
	if !p.addOpen {
		return
	}
	close := func() {
		p.addOpen = false
		if p.addKind == "yggdrasil" {
			p.clearExternal()
			actions.ExternalClose()
			return
		}
		if data.Busy {
			actions.Cancel()
		}
	}
	title := locales.AddOfflineAccount()
	if p.addKind == "" {
		title = locales.AddAccount()
	}
	if p.addKind == "microsoft" {
		title = locales.SignInWithMicrosoft()
	}
	if p.addKind == "yggdrasil" {
		title = locales.ExternalLogin()
	}
	c.Key("add-account")
	dialog := kit.Dialog(c, &p.addOpen, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, title, close).Label(locales.CloseAddAccount())
			kit.SlideMotion(ui.Column(c.Key("account-kind:"+p.addKind)).FillWidth().MinHeight(0).Shrink(1).Gap(16), p.addKind != "").Children(func() {
				if p.addKind == "" {
					for _, option := range []struct{ kind, label string }{{"microsoft", locales.MicrosoftAccount()}, {"offline", locales.OfflineAccount()}, {"yggdrasil", locales.ExternalLogin()}} {
						kit.ListRow(c, kit.Row{Key: option.kind, Title: option.label, OnClick: func() { p.chooseKind(option.kind, actions) }}, nil)
					}
					return
				}
				if data.Error != "" {
					kit.ErrorText(c, data.Error).Wrap()
				}
				if p.addKind == "microsoft" {
					p.microsoftDialog(c, data, actions)
					return
				}
				if p.addKind == "yggdrasil" {
					p.externalDialog(c, data, actions)
					return
				}
				add := func() {
					if data.Locked || data.Busy || strings.TrimSpace(p.name) == "" {
						return
					}
					if actions.AddOffline(p.name) {
						p.name = ""
						p.addOpen = false
					}
				}
				ui.Column(c).FillWidth().Gap(8).Children(func() {
					kit.Label(c, locales.PlayerName(), kit.Body).TextColor(kit.Muted)
					c.Key("offline-name")
					kit.Input(c, &p.name).FillWidth().Height(kit.ControlH()).Placeholder(locales.EnterPlayerName()).Label(locales.OfflinePlayerName()).Disabled(data.Locked || data.Busy).OnSubmit(add)
				})
				kit.DialogFooter(c, func() {
					kit.Primary(c, locales.Add(), add).Disabled(data.Locked || data.Busy || strings.TrimSpace(p.name) == "")
				})
			})
		})
	})
	if dialog.Dismissed() {
		close()
	}
}

func (p *Accounts) microsoftDialog(c *ui.Context, data AccountData, actions AccountActions) {
	services := c.Services()
	if data.Busy {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c)
			kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).Wrap()
		})
		if data.Code != "" {
			ui.Column(c).FillWidth().Padding(16).Gap(8).Radius(kit.RadiusL).Background(kit.SurfaceHi).Children(func() {
				kit.Label(c, locales.SignInCode(), kit.Body).TextColor(kit.Muted)
				ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
					kit.Label(c, data.Code, kit.Body).Grow(1)
					kit.Button(c, locales.Copy(), func() { services.WriteClipboard(data.Code) }).Label(locales.CopySignInCode())
				})
			})
			kit.Primary(c, locales.OpenSignInPage(), func() { services.OpenURL(data.URL) }).FillWidth().Height(kit.ControlH())
		}
		return
	}
	ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
		kit.Primary(c, locales.Retry(), func() { actions.Login(); p.waiting = true }).Disabled(data.Locked)
	})
}
