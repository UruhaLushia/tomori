package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type AccountData struct {
	Accounts         config.Accounts
	TopNavigation    bool
	Avatar           func(string) *ui.Bitmap
	Locked           bool
	Busy             bool
	Code             string
	URL              string
	Status           string
	Error            string
	ExternalServer   account.YggdrasilServer
	ExternalProfiles []account.YggdrasilProfile
}

type AccountActions struct {
	AddOffline      func(string) bool
	Login           func()
	Select          func(string)
	Remove          func(string)
	Cancel          func()
	ClearError      func()
	Appearance      func(string)
	ExternalBegin   func(string)
	ExternalInspect func(string)
	ExternalLogin   func(string, string)
	ExternalSelect  func(string)
	ExternalClose   func()
	ExternalBack    func()
	SaveServer      func(string, string) bool
	RemoveServer    func(string)
}

type Accounts struct {
	name                                                                     string
	addAction                                                                string
	addKind                                                                  string
	addOpen, waiting                                                         bool
	list                                                                     ui.ListState
	externalURL, externalName, externalPassword, externalProfile, externalID string
	serversOpen                                                              bool
	serverName, serverURL                                                    string
}

func (p *Accounts) View(c *ui.Context, data AccountData, actions AccountActions) {
	if !data.TopNavigation {
		kit.PageTitle(c, locales.AccountsPage(), "", func() { p.addMenu(c, data, actions) })
	}
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).Gap(kit.Gap(c)).Children(func() {
		if data.Error != "" && !p.addOpen {
			kit.ErrorText(c, data.Error)
		}
		kit.FormGroup(c, locales.AccountsPage(), func() {
			if data.TopNavigation {
				p.addMenu(c, data, actions)
			}
		}, func() {
			if len(data.Accounts.Entries) == 0 {
				p.list = ui.ListState{}
				ui.Box(c).FillWidth().Padding(24).Children(func() {
					kit.EmptyState(c, kit.User, locales.NoAccountsAdded(), locales.AddAMicrosoftOfflineOrThirdPartyAccountAbove(), nil)
				})
				return
			}
			c.Key("account-results")
			kit.PresenceMotion(ui.Column(c).FillWidth().Margin(4, -8).Children(func() {
				for _, identity := range data.Accounts.Entries {
					c.Key(identity.ID)
					p.accountRow(c, identity, data, actions)
				}
			}))
		})
		kit.FormGroup(c, locales.ExternalLogin(), nil, func() {
			kit.FormValueRow(c, locales.AuthenticationServers(), fmt.Sprintf(locales.Servers2(), len(data.Accounts.Servers)), func() {
				kit.Button(c, locales.ManageServers(), func() { p.serversOpen = true }).Disabled(data.Locked || data.Busy)
			})
		})
	})
}

func (p *Accounts) AddOpen() bool { return p.addOpen }
func (p *Accounts) ShowAdd()      { p.addKind, p.addOpen = "", true }
func (p *Accounts) Dialogs(c *ui.Context, data AccountData, actions AccountActions) {
	if p.waiting && !data.Busy {
		p.waiting = false
		if data.Error == "" && len(data.ExternalProfiles) == 0 {
			p.addOpen = false
			p.clearExternal()
		}
	}
	p.addDialog(c, data, actions)
	p.serversDialog(c, data, actions)
}

func (p *Accounts) chooseKind(kind string, actions AccountActions) {
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
}

func (p *Accounts) Reauthenticate(identity account.Account, actions AccountActions) {
	if identity.Kind == account.Yggdrasil {
		p.openExternal(identity)
		actions.ExternalBegin(identity.ID)
		return
	}
	p.chooseKind("microsoft", actions)
}

func (p *Accounts) accountRow(c *ui.Context, identity account.Account, data AccountData, actions AccountActions) {
	selected := identity.ID == data.Accounts.Selected
	row := kit.LayoutMotion(ui.Row(c).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusS).AlignItems(ui.Center))
	row.Label(locales.Account() + identity.Name)
	row.Children(func() {
		var avatar *ui.Bitmap
		if data.Avatar != nil {
			avatar = data.Avatar(identity.ID)
		}
		kit.PlayerAvatar(c, avatar, identity.Initial(), 32)
		ui.Column(c).Grow(1).MinWidth(0).Gap(0).Children(func() {
			kit.Line(c, identity.Name, kit.Strong).MaxLines(1).Ellipsis("…")
			kit.Line(c, identity.Label(), kit.Caption).MaxLines(1).Ellipsis("…")
		})
		ui.Row(c).Width(kit.W(104)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			switch {
			case selected:
				kit.Badge(c, locales.Current(), kit.AccentTone)
			case row.Hovered() && identity.Kind == account.Yggdrasil:
				kit.Ghost(c, locales.SignInAgain(), func() { p.openExternal(identity); actions.ExternalBegin(identity.ID) }).Height(kit.TabH()).Disabled(data.Locked || data.Busy)
			default:
				kit.Ghost(c, locales.Use(), func() { actions.Select(identity.ID) }).Height(kit.TabH()).Disabled(data.Locked || data.Busy)
			}
			kit.ContextMenuButton(c, row, locales.MoreAccountActions())
		})
	})
	kit.ContextMenu(c, row, func(menu *kit.ContextActions) {
		menu.Item(c, "select", locales.Use(), data.Locked || data.Busy || selected, func() { actions.Select(identity.ID) })
		if identity.Kind == account.Microsoft {
			menu.Item(c, "appearance", locales.SkinAndCape(), data.Locked || data.Busy, func() { actions.Appearance(identity.ID) })
		}
		if identity.Kind == account.Yggdrasil {
			menu.Item(c, "login", locales.SignInAgain(), data.Locked || data.Busy, func() { p.openExternal(identity); actions.ExternalBegin(identity.ID) })
		}
		menu.Separator(c)
		menu.Item(c, "delete", locales.Delete(), data.Locked || data.Busy, func() { actions.Remove(identity.ID) }, kit.DangerTone)
	})
}

func (p *Accounts) serversDialog(c *ui.Context, data AccountData, actions AccountActions) {
	if !p.serversOpen {
		return
	}
	dialog := kit.Dialog(c, &p.serversOpen, kit.DialogM, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, locales.AuthenticationServers(), func() { p.serversOpen = false })
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				kit.Input(c, &p.serverName).Height(kit.ControlH()).FillWidth().Placeholder(locales.ServerNameOptional()).Label(locales.ServerName())
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					kit.Input(c, &p.serverURL).Height(kit.ControlH()).Grow(1).MinWidth(0).Placeholder("https://").Label(locales.AuthenticationServerAddress())
					kit.Button(c, locales.Add(), func() {
						if actions.SaveServer != nil && actions.SaveServer(p.serverName, p.serverURL) {
							p.serverName, p.serverURL = "", ""
						}
					}).Disabled(data.Locked || data.Busy || p.serverURL == "")
				})
			})
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			ui.Scroll(c).Scrollbars(false).FillWidth().MaxHeight(320).Children(func() {
				ui.Column(c).FillWidth().Children(func() {
					for _, server := range data.Accounts.Servers {
						c.Key(server.URL)
						used := false
						for _, identity := range data.Accounts.Entries {
							if identity.ServerURL == server.URL {
								used = true
								break
							}
						}
						kit.ListRow(c, kit.Row{Title: server.Name, Subtitle: kit.MiddleEllipsis(server.URL, 64), Lead: func() { kit.IconBadge(c, kit.Cube, 32) }}, func(bool) {
							kit.Ghost(c, locales.SignIn(), func() {
								p.serversOpen = false
								p.openExternal(account.Account{ServerURL: server.URL})
								actions.ExternalBegin("")
							}).Height(kit.TabH()).Disabled(data.Locked || data.Busy)
							kit.IconButton(c, kit.Trash, false, func() {
								if actions.RemoveServer != nil {
									actions.RemoveServer(server.URL)
								}
							}).Size(28, 28).Label(locales.RemoveServer()).Tooltip(locales.RemoveServer()).Disabled(data.Locked || data.Busy || used)
						})
					}
				})
			})
		})
	})
	if dialog.Dismissed() {
		p.serversOpen = false
	}
}
