package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type ComponentsData struct {
	Open, Loading, Saving  bool
	Key, Name, Game, Error string
	Current                minecraft.InstanceComponents
	Versions               []minecraft.VersionSummary
	Loaders                LoaderCatalogData
}

type ComponentsActions struct {
	Close, Retry, Manifest func()
	Loaders                func(string, string)
	Save                   func(string, modpack.Loader)
}

type Components struct {
	initialized bool
	gameList    ui.ListState
	gameFilter  InstallDialog
	editor      InstallDialog
}

func (p *Components) View(c *ui.Context, data *ComponentsData, actions ComponentsActions) {
	c.Key("instance-components")
	if data.Open && !data.Loading && data.Current.Game != "" && !p.initialized {
		p.initialized = true
		p.editor.loader, p.editor.loaderVersion = data.Current.Loader, data.Current.LoaderVersion
		if data.Current.Reason == "" {
			actions.Loaders(data.Game, p.editor.loader)
		}
	}
	dialog := kit.Dialog(c, &data.Open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, locales.InstanceComponents(), actions.Close)
			kit.Label(c, data.Name, kit.Body).TextColor(kit.Muted).SingleLine().Ellipsis("…")
			if data.Loading {
				ui.Row(c).FillWidth().Height(120).Center().Children(func() { ui.Spinner(c) })
				return
			}
			if len(data.Current.Items) > 0 {
				ui.Scroll(c).Scrollbars(false).FillWidth().Height(float32(min(len(data.Current.Items), 5)) * kit.RowSingleH()).Children(func() {
					ui.Column(c).FillWidth().Shrink(0).Padding(0, kit.ScrollbarGutter, 0, 0).Children(func() {
						for i, component := range data.Current.Items {
							if i > 0 {
								kit.Divider(c)
							}
							ui.Row(c).FillWidth().Padding(12, 0).Gap(12).AlignItems(ui.Center).Children(func() {
								kit.Label(c, component.Name, kit.Body).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
								if component.Disabled {
									kit.Pill(c, locales.Disabled2(), kit.Dim)
								}
								kit.Label(c, component.Version, kit.Body).TextColor(kit.Muted).SingleLine().Ellipsis("…")
							})
						}
					})
				})
			}
			if data.Current.Reason != "" {
				kit.ErrorText(c, data.Current.Reason)
			} else if data.Current.Game != "" {
				kit.Divider(c)
				p.gameInput(c, data, actions)
				if len(p.editor.filtered) == 0 {
					p.editor.filtered = []minecraft.VersionSummary{{ID: data.Game}}
				} else {
					p.editor.filtered[0].ID = data.Game
				}
				p.editor.loaderInput(c, InstancesData{Loaders: data.Loaders, Busy: data.Saving}, InstancesActions{Loaders: actions.Loaders})
				kit.Label(c, locales.InstalledModsMayNoLongerBeCompatibleAfterChangingTheGameVersionOrLoader(), kit.Body).TextColor(kit.Muted).Wrap()
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
				if data.Current.Game == "" {
					kit.Button(c, locales.Retry(), actions.Retry)
				}
			}
			if data.Current.Game != "" && data.Current.Reason == "" {
				kit.DialogFooter(c, func() {
					if data.Saving {
						ui.Spinner(c)
						kit.Label(c, locales.PreparingComponents(), kit.Body).TextColor(kit.Muted)
					}
					ui.Spacer(c)
					changed := data.Game != data.Current.Game || p.editor.loader != data.Current.Loader || p.editor.loader != "" && p.editor.loaderVersion != data.Current.LoaderVersion
					kit.Primary(c, locales.ApplyChanges(), func() { actions.Save(data.Game, modpack.Loader{ID: p.editor.loader, Version: p.editor.loaderVersion}) }).
						Disabled(data.Saving || !changed || !p.editor.loaderReady(data.Loaders, data.Game))
				})
			}
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}

func (p *Components) gameInput(c *ui.Context, data *ComponentsData, actions ComponentsActions) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		kit.Label(c, locales.GameVersion(), kit.Body).TextColor(kit.Muted)
		c.Key("component-game")
		input := kit.SelectBase(c, &data.Game)
		kit.SelectTrigger(c, input.Trigger, input.Open()).Grow(1).MinWidth(0).Disabled(data.Saving).Label(locales.SelectGameVersion()).OnChange(func() {
			p.editor.loaderVersion = ""
			actions.Loaders(data.Game, p.editor.loader)
		}).Children(func() {
			kit.Label(c, data.Game, kit.Body).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
			ui.Icon(c, kit.ChevronRight).FontSize(kit.IconPx(12)).Rotate(90).TextColor(kit.Muted)
		})
		input.Popup(func(panel ui.Element) {
			kit.PopupPanel(c, panel, 320, input.Trigger)
			kit.SearchInput(c, &p.gameFilter.query, locales.SearchVersions())
			p.gameFilter.filterVersions(data.Versions)
			if len(data.Versions) == 0 {
				kit.Button(c, locales.LoadVersionList(), actions.Manifest)
			}
			ui.List(c, &p.gameList, len(p.gameFilter.filtered), func(index int) {
				v := p.gameFilter.filtered[index]
				c.Key(v.ID)
				kit.MenuItem(c, input.Item(v.ID), v.ID)
			}).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0).Scrollbars(false).Height(min(240, float32(len(p.gameFilter.filtered)*34)))
		})
	})
}
