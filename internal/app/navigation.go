package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	pageHome = iota
	pageInstances
	pageAccount
	pageGame
	pageJava
	pageGeneral
	pageDownload
	pageNetwork
	pageAppearance
	pageLogs
	pageAbout
)

const sidebarWidth = 144

type navigationItem struct {
	page int
	name func() string
}

var sections = []struct {
	name  func() string
	page  int
	items []navigationItem
}{
	{locales.HomePage, pageHome, nil},
	{locales.Library, pageInstances, nil},
	{locales.DownloadsPage, pageDownload, nil},
	{locales.Settings, pageGame, []navigationItem{
		{pageGame, locales.Game},
		{pageJava, locales.Java},
		{pageAccount, locales.AccountsPage},
		{pageNetwork, locales.Network},
		{pageGeneral, locales.General},
		{pageAppearance, locales.Appearance},
	}},
	{locales.More, pageLogs, []navigationItem{{pageLogs, locales.Logs}, {pageAbout, locales.About}}},
}

var sidebarAnimation = kit.MotionNavRail
var sidebarCloseAnimation = kit.MotionNavRail
var sidebarFadeAnimation = kit.MotionFast

func sectionOf(page int) int {
	for index, section := range sections {
		if section.page == page {
			return index
		}
		for _, item := range section.items {
			if item.page == page {
				return index
			}
		}
	}
	return 0
}

func pageName(page int) string {
	for _, section := range sections {
		if section.page == page {
			return section.name()
		}
		for _, item := range section.items {
			if item.page == page {
				return item.name()
			}
		}
	}
	return ""
}

func pageOrder(page int) int {
	order := 0
	for _, section := range sections {
		if section.page == page {
			return order
		}
		order++
		for _, item := range section.items {
			if item.page == page {
				return order
			}
			order++
		}
	}
	return order
}

func sectionIcon(section int) *ui.SVG {
	switch section {
	case 0:
		return kit.Home
	case 1:
		return kit.Cube
	case 2:
		return kit.Download
	case 3:
		return kit.Sliders
	}
	return kit.Grid
}

func (a *app) sidebarOpen() bool {
	return a.cfg.Appearance.Navigation == config.NavSidebar || a.cfg.Appearance.Navigation == config.NavIconSidebar
}

func (a *app) sidebar(c *ui.Context) {
	if !a.sidebarOpen() {
		return
	}
	if a.cfg.Appearance.Navigation == config.NavIconSidebar {
		c.Key("navigation-rail")
		ui.Column(c).Width(48).Shrink(0).FillHeight().Padding(8).Gap(4).Radius(kit.RadiusL).Border(1, kit.LineSoft).Background(kit.Surface).Children(func() {
			for i, section := range sections {
				c.Key(i)
				kit.SideNavIconButton(c, sectionIcon(i), a.section == i, func() { a.goPage(section.page) }).Size(32, 32).Label(section.name()).Tooltip(section.name()) // nav:shell
			}
		})
		return
	}
	width := float32(sidebarWidth)
	if strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") {
		width = 184
	}
	for _, section := range sections {
		w, _ := c.MeasureText(1<<20, ui.Span{Text: section.name(), Size: kit.FontPx(kit.Strong), Weight: 600})
		width = max(width, w+56)
		for _, item := range section.items {
			w, _ := c.MeasureText(1<<20, ui.Span{Text: item.name(), Size: kit.FontPx(kit.Strong), Weight: 600})
			width = max(width, w+56)
		}
	}
	el := ui.Column(c.Key("navigation-sidebar")).Width(width).Shrink(0).FillHeight().Radius(kit.RadiusL).Border(1, kit.LineSoft).Clip().Background(kit.Surface)
	el.Children(func() {
		rowHeight := kit.ControlH()
		height := 16 + float32(len(sections))*(rowHeight+4) - 4
		if a.openSection >= 0 && a.openSection < len(sections) {
			height += sidebarMenuHeight(a.openSection, rowHeight)
		}
		ui.Scroll(c).Scrollbars(false).Fill().Children(func() {
			ui.Box(c).FillWidth().Height(height).Shrink(0).Padding(8).Children(func() {
				y := float32(0)
				for si := range sections {
					animation := sidebarCloseAnimation
					if a.openSection >= 0 && a.openSection < si {
						animation = sidebarAnimation
					}
					c.Key(si)
					ui.Column(c).Absolute().Top(8).Left(8).Right(8).Height(rowHeight).
						AnimatePose(ui.Pose{Y: y}, kit.LayoutAnimation(animation)).
						Children(func() { a.sectionRow(c, si, rowHeight) })
					y += rowHeight + 4
					if a.openSection == si {
						y += sidebarMenuHeight(si, rowHeight)
					}
				}
			})
		})
	})
}

func sidebarMenuHeight(si int, rowHeight float32) float32 {
	return float32(len(sections[si].items)) * (kit.TabH() + 4)
}

func (a *app) sectionRow(c *ui.Context, si int, rowHeight float32) {
	s := sections[si]
	open := a.openSection == si
	active := a.section == si && len(s.items) == 0
	row := kit.NavRow(c, sectionIcon(si), s.name(), active, func() {
		if a.section != si || len(s.items) == 0 {
			a.goPage(s.page) // nav:shell
		}
	}).Height(rowHeight).Shrink(0)
	if len(s.items) > 0 {
		row.Expanded(open)
	}
	if len(s.items) > 0 {
		height := sidebarMenuHeight(si, rowHeight)
		y := -height
		animation := sidebarCloseAnimation
		if open {
			y = 0
			animation = sidebarAnimation
		}
		ui.Box(c).Absolute().Top(rowHeight).Left(0).Right(0).Height(height).ClipY().PassThrough().
			AnimatePose(ui.Pose{Y: y}, kit.LayoutAnimation(animation)).Children(func() {
			ui.Column(c).FillWidth().Height(height).AnimatePose(ui.Pose{Y: -y}, kit.LayoutAnimation(animation)).Children(func() {
				selected := -1
				for index, item := range s.items {
					if a.page == item.page {
						selected = index
					}
				}
				kit.NavSubGroup(c, selected, func() {
					for index, item := range s.items {
						c.Key(item.page)
						itemRow := kit.NavSubRow(c, item.name(), a.page == item.page, func() {
							if open {
								a.goPage(item.page) // nav:shell
							}
						}).Shrink(0)
						itemPose := ui.Pose{Y: -4, Fade: 0}
						itemAnimation := sidebarCloseAnimation
						if open {
							itemPose = ui.Pose{}
							itemAnimation = sidebarAnimation
							itemAnimation.Delay = time.Duration(index) * 16 * time.Millisecond
						}
						itemRow.AnimatePose(itemPose, kit.LayoutAnimation(itemAnimation))
					}
				}).Margin(4, 0, 0, 0).AnimatePose(ui.Pose{Fade: boolFloat(!open)}, kit.LayoutAnimation(sidebarFadeAnimation))
			})
		})
	}
}

func (a *app) goPage(page int) {
	if page == pageInstances {
		a.closeInstanceDetail()
	}
	if a.page == page {
		return
	}
	a.pageDirection = 1
	if pageOrder(page) < pageOrder(a.page) {
		a.pageDirection = -1
	}
	if a.page == pageHome {
		a.pages.home.CloseSwitcher()
		a.cancelHomeScan()
		a.home.loadedAt = time.Time{}
	}
	if a.page == pageInstances {
		a.pages.instances.CancelInteractions()
		a.cancelHomeScan()
		a.icons.invalidate()
		a.closePackAssociate()
		a.releaseServers()
		a.closeWorldBackups()
		a.closeScreenshotEdit()
		a.releaseInstanceMedia()
	}
	if a.page == pageLogs {
		a.releaseLogHistory()
	}
	if a.page == pageAppearance {
		a.cancelZoomOutline()
	}
	if a.page == pageAccount {
		a.closeAppearance()
		a.closeExternalAuth()
	}
	if a.resources.data.Open && (a.resources.data.Embedded || a.resources.data.FullPage && page != pageDownload) {
		a.closeResources()
	}
	if page == pageAppearance {
		a.loadFonts()
	}
	a.page = page
	a.sectionScroll = ui.ScrollState{}
	a.section = sectionOf(page)
	if len(sections[a.section].items) > 0 {
		a.openSection = a.section
	} else {
		a.openSection = -1
	}
}

func (a *app) closeInstanceDetail() {
	if !a.pages.instances.CanGoBack() {
		return
	}
	a.pages.instances.CloseDetail()
	a.resetInstanceContent()
	a.closeResources()
	a.releaseServers()
}

func boolFloat(on bool) float32 {
	if on {
		return 1
	}
	return 0
}
