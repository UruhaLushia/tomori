package page

import (
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
)

type screenshotOption struct {
	id, label string
	disabled  bool
}

func screenshotOptions(locked bool) [5]screenshotOption {
	return [5]screenshotOption{{"open", locales.OpenExternally(), false}, {"directory", locales.OpenFolder(), false}, {"path", locales.CopyPath(), false}, {"export", locales.ExportScreenshot(), locked}, {"delete", locales.MoveToTrash(), locked}}
}

func screenshotAction(file instancecontent.File, mode string, actions InstancesActions, clipboard func(string)) {
	switch mode {
	case "open":
		actions.OpenFile(file.Path)
	case "directory":
		actions.Open(filepath.Dir(file.Path))
	case "path":
		clipboard(file.Path)
	case "export", "delete":
		actions.EditScreenshot(file, mode)
	}
}

func screenshotContext(c *ui.Context, element ui.Element, file instancecontent.File, locked bool, actions InstancesActions) {
	services := c.Services()
	kit.ContextMenu(c, element, func(menu *kit.ContextActions) {
		menu.Item(c, "preview", locales.Preview(), false, func() { actions.Preview(file.Path) })
		for _, option := range screenshotOptions(locked) {
			if option.id == "delete" {
				menu.Separator(c)
			}
			menu.Item(c, option.id, option.label, option.disabled, func() { screenshotAction(file, option.id, actions, services.WriteClipboard) })
		}
	})
}

func screenshotMenu(c *ui.Context, file instancecontent.File, locked bool, actions InstancesActions) {
	c.Key("screenshot-menu")
	holder := ui.Row(c).Shrink(0)
	value := ui.Local(holder, "action", func() string { return "" })
	services := c.Services()
	holder.Children(func() {
		menu := kit.SelectBase(c, value)
		kit.MenuButton(c, menu.Trigger, locales.Manage(), menu.Open()).Label(locales.ScreenshotActions()).OnChange(func() {
			mode := *value
			*value = ""
			screenshotAction(file, mode, actions, services.WriteClipboard)
		})
		menu.Popup(func(panel ui.Element) {
			kit.PopupPanel(c, panel, 168, menu.Trigger).Role(ui.RoleMenu)
			for _, option := range screenshotOptions(locked) {
				if option.id == "delete" {
					kit.Divider(c)
				}
				c.Key(option.id)
				kit.MenuItem(c, menu.Item(option.id).Disabled(option.disabled), option.label)
			}
		})
	})
}
