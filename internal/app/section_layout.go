package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
)

// topSectionLayout keeps secondary pages in the page header when the primary
// navigation is in the top bar. The content below it remains a single scroll
// region so changing a tab never moves the shell or the window controls.
func (a *app) topSectionLayout(c *ui.Context) {
	pad := kit.Pad(c)
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		title := pageName(a.page)
		var tabs func()
		if a.cfg.Appearance.Navigation != config.NavSidebar {
			title = sections[a.section].name()
			tabs = func() {
				ui.Row(c).Wrap().Gap(4).AlignItems(ui.Center).Children(func() {
					for _, item := range sections[a.section].items {
						c.Key(item.page)
						kit.NavTab(c, item.name(), a.page == item.page, func() { a.goPage(item.page) }) // nav:shell
					}
				})
			}
		}
		var actions func()
		if a.page == pageAccount {
			actions = func() { a.dialogs.accounts.AddMenu(c, a.accountData(), a.accountActions(c.Services())) }
		}
		ui.Box(c).FillWidth().Margin(pad, pad, 0, pad).Shrink(0).Children(func() {
			kit.FormColumn(c, func() { kit.PageHeader(c, title, "", tabs, actions) })
		})
		scroll := ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).TrackScroll(&a.sectionScroll)
		if a.sectionScroll.Y > 0 {
			kit.ScrolledEdge(scroll)
		}
		scroll.Children(func() {
			ui.Column(c).FillWidth().Shrink(0).Padding(12, pad, pad, pad).Children(func() {
				kit.FormColumn(c, func() {
					c.Key(a.page)
					a.pageContent(c)
				})
			})
		})
	})
}
