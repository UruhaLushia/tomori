package page

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type PackAssociateData struct {
	Open, Loading, Busy, Confirmed, Optional      bool
	Key, Name, URL, Selected, Path, Error, Status string
	Versions                                      []packcatalog.Version
	Info                                          modpack.Info
}

type PackAssociateActions struct {
	Close, Read, Changed, Browse, Save func()
	Choose                             func(packcatalog.Version)
}

type PackAssociate struct{ versions ui.ListState }

func (p *PackAssociate) View(c *ui.Context, data *PackAssociateData, actions PackAssociateActions) {
	if !data.Open {
		return
	}
	c.Key("pack-associate:" + data.Key)
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(_, panel ui.Element) {
		panel.Height(560).Gap(8)
		panel.Children(func() {
			kit.DialogHeader(c, locales.LinkModpack(), actions.Close)
			kit.Label(c, data.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Busy {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					ui.Spinner(c)
					kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				})
				return
			}
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				c.Key("url")
				kit.Input(c, &data.URL).Grow(1).MinWidth(0).Placeholder(locales.ModpackProjectOrVersionPage()).Label(locales.ModpackUrl()).Disabled(data.Loading).OnChange(actions.Changed).OnSubmit(actions.Read)
				kit.Button(c, locales.Read(), actions.Read).Disabled(data.Loading || strings.TrimSpace(data.URL) == "")
			})
			if data.Loading {
				ui.Spinner(c).Size(16, 16)
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if len(data.Versions) == 0 {
				kit.Label(c, locales.LinkToAnOnlineModpackToEnableUpdateChecks(), kit.Body).TextColor(kit.Muted)
				ui.Spacer(c)
				return
			}
			kit.Label(c, locales.InstalledVersion(), kit.Body)
			ui.List(c, &p.versions, len(data.Versions)).Grow(1).MinHeight(40).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any { return data.Versions[i].ID }).Scrollbars(false).Rows(func(row ui.ListRow) {
				v := data.Versions[row.Index]
				c := row.Context
				button := kit.RowButton(c, "", data.Selected == v.ID, func() { actions.Choose(v) }).FillWidth().Padding(8, 12).Justify(ui.Start).Disabled(data.Loading)
				if data.Selected == v.ID {
					button.Background(kit.AccentSoft)
				} else if button.Hovered() {
					button.Background(kit.Hover)
				}
				button.Children(func() {
					ui.Column(c).FillWidth().Gap(4).Children(func() {
						kit.Label(c, v.Name, kit.Body).MaxLines(1).Ellipsis("…")
						kit.Label(c, v.Number, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					})
				})
			})
			if data.Selected != "" {
				if data.Info.Format != "" {
					label := "Minecraft " + packVersion(data.Info.GameVersion)
					if loader := data.Info.LoaderLabel(); loader != "" {
						label += " · " + loader
					}
					kit.Label(c, label, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				}
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					label := locales.SelectTheArchiveForThisVersion()
					if data.Path != "" {
						label = filepath.Base(data.Path)
					}
					kit.Label(c, label, kit.Body).Grow(1).MinWidth(0).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					kit.Button(c, locales.ChooseFiles(), actions.Browse).Disabled(data.Loading)
				})
				ui.Checkbox(c, &data.Optional, locales.IncludeOptionalFiles()).Disabled(data.Loading)
				ui.Checkbox(c, &data.Confirmed, locales.ThisIsTheCurrentlyInstalledVersion()).Disabled(data.Loading || data.Info.Format == "")
			}
			kit.DialogFooter(c, func() {
				kit.Primary(c, locales.Link(), actions.Save).Disabled(data.Loading || !data.Confirmed || data.Info.Format == "")
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
