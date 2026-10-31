package app

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) topBar(c *ui.Context) {
	height, pad := kit.H(42, kit.Body), float32(12)
	w, _ := c.Size()
	needed := float32(260)
	for _, s := range sections {
		needed += kit.TextWidth(c, s.name(), kit.Strong) + 48
	}
	iconsOnly := needed > w
	c.Key("window-topbar")
	bar := ui.Row(c).Height(height).Shrink(0).FillWidth().Padding(0, pad, 0, 16).Gap(8).
		Background(kit.Window).
		AlignItems(ui.Center)
	bar.DragWindow()
	bar.Children(func() {
		c.Key("window-title")
		kit.Label(c, locales.Tomori(), kit.Strong)

		if a.cfg.Appearance.Navigation == config.NavTop {
			c.Key("top-navigation-gap")
			ui.Box(c).Width(4)
			c.Key("top-navigation")
			nav := ui.Row(c).Gap(4).AlignItems(ui.Center)
			nav.Children(func() {
				for i := range sections {
					section := i
					c.Key("top-nav-" + strconv.Itoa(i))
					if iconsOnly {
						kit.IconButton(c, sectionIcon(i), a.section == i, func() { a.goPage(sections[section].page) }).Size(32, 32).Label(sections[i].name()).Tooltip(sections[i].name()) // nav:shell
						continue
					}
					kit.NavPill(c, sectionIcon(i), sections[i].name(), a.section == i, func() {
						a.goPage(sections[section].page) // nav:shell
					}).Shrink(0).Height(kit.TabH()).Padding(0, 8).Gap(8)
				}
			})
		}

		c.Key("topbar-spacer")
		ui.Spacer(c)
		c.Key("topbar-actions")
		a.titleBarActions(c)
	})
}

func (a *app) titleBarActions(c *ui.Context) {
	ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
		a.packRecoveryButton(c)
		c.Key("download-slot")
		ui.Box(c).Size(28, 28).Shrink(0).Children(func() { a.downloadButton(c) })
		themeIcon, themeLabel := kit.Moon, locales.SwitchToDarkMode()
		if kit.Dark() {
			themeIcon, themeLabel = kit.Sun, locales.SwitchToLightMode()
		}
		c.Key("theme-toggle")
		kit.IconButton(c, themeIcon, false, a.toggleTheme).Size(28, 28).Label(themeLabel)
		c.Key("window-controls")
		ui.Row(c).Shrink(0).Gap(4).AlignItems(ui.Center).Children(func() {
			kit.WindowButton(c, kit.Minus, false, a.minimize).Size(28, 28).Label(locales.Minimize())
			kit.WindowButton(c, kit.Square, false, a.maximize).Size(28, 28).Label(locales.MaximizeRestore())
			kit.WindowButton(c, kit.Close, true, a.closeWindow).Size(28, 28).Label(locales.Close())
		})
	})
}

func (a *app) toggleTheme() {
	a.cfg.Appearance.ColorMode = config.ColorDark
	if kit.Dark() {
		a.cfg.Appearance.ColorMode = config.ColorLight
	}
	a.appearanceChanged()
}

func (a *app) minimize() {
	if a.win != nil {
		go a.win.Minimize()
	}
}

func (a *app) maximize() {
	if a.win != nil {
		go a.win.ToggleMaximize()
	}
}

func (a *app) closeWindow() {
	if a.win != nil {
		go a.win.Close()
	}
}
