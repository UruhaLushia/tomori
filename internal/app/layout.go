package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
)

func (a *app) view(c *ui.Context) {
	a.applyZoom(c)
	a.applyAppearance(c.Theme().Dark)
	c.SetTheme(kit.Theme())
	defer kit.BeginFrame(c)()
	root := kit.Body.Apply(ui.Box(c)).Fill().Background(kit.Window)
	root.Children(func() {
		ui.Column(c).Fill().Children(func() {
			a.topBar(c)
			pad := float32(12)
			ui.Row(c).Grow(1).MinWidth(0).MinHeight(0).Padding(0, pad, pad, pad).Gap(8).Children(func() {
				a.sidebar(c)
				a.content(c)
			})
		})
		a.fpsMeter(c)
	})
	a.dialogsView(c)
	a.resourcesView(c)
	a.resourceImportView(c)
	a.directoryEditorView(c)
	a.packImportView(c)
	a.instanceSettingsView(c)
	a.instanceEditView(c)
	a.instanceCopyView(c)
	a.worldEditView(c)
	a.worldArchiveView(c)
	a.worldBackupsView(c)
	a.screenshotEditView(c)
	a.serverEditView(c)
	a.appearanceView(c)
	a.logDiagnosticView(c)
	a.packExportView(c)
	a.packUpdateView(c)
	a.packAssociateView(c)
	a.packRestoreView(c)
	a.packRecoveryView(c)
	a.componentsView(c)
	a.notificationsView(c)
	if a.page == pageInstances && a.pages.instances.CanGoBack() {
		c.OnShortcut(0, ui.KeyEscape, a.closeInstanceDetail)
	}
}

func (a *app) content(c *ui.Context) {
	ui.Column(c.Key("page-content")).Grow(1).MinWidth(0).MinHeight(0).FillHeight().Gap(0).Clip().Children(func() {
		page := ui.Box(c).Grow(1).FillWidth()
		page.Children(func() {
			a.pagePanel(c)
		})
	})
}

func (a *app) pagePanel(c *ui.Context) {
	frame := ui.Box(c).Fill()
	frame.Children(func() {
		panel := ui.Column(c).Fill().MinWidth(0).MinHeight(0).Background(kit.Surface).Border(1, kit.LineSoft).Radius(kit.RadiusL).Clip()
		panel.Children(func() {
			page := ui.Column(c.Key(a.page)).Fill().MinWidth(0).MinHeight(0)
			kit.NavigationPageMotion(page, a.cfg.Appearance.Navigation == config.NavTop, a.pageDirection).Children(func() { a.sectionPanel(c) })
		})
	})
}

func (a *app) sectionPanel(c *ui.Context) {
	if a.page == pageHome {
		a.homePage(c)
		return
	}
	if a.page == pageInstances {
		a.instancesPage(c)
		return
	}
	if len(sections[a.section].items) > 0 && a.page != pageLogs {
		a.topSectionLayout(c)
		return
	}
	kit.Page(c, func() {
		a.pageContent(c)
	})
}

func (a *app) pageContent(c *ui.Context) {
	switch a.page {
	case pageInstances:
		a.instancesPage(c)
	case pageAccount:
		a.accountPage(c)
	case pageGame:
		a.gamePage(c)
	case pageJava:
		a.javaPage(c)
	case pageGeneral:
		a.generalPage(c)
	case pageDownload:
		a.downloadPage(c)
	case pageNetwork:
		a.networkPage(c)
	case pageAppearance:
		a.appearancePage(c)
	case pageLogs:
		a.logsPage(c)
	case pageAbout:
		a.aboutPage(c)
	}
}
