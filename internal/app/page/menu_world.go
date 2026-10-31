package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type worldOption struct {
	mode, label string
	disabled    bool
}

func worldEditOptions(world instancecontent.World, locked bool) [6]worldOption {
	return [6]worldOption{
		{"datapacks", locales.DataPacks(), locked || !world.Valid},
		{"rename", locales.Rename(), locked || !world.Valid},
		{"copy", locales.CopyWorld(), locked || !world.Valid},
		{"backup", locales.BackUpWorld(), locked || !world.Valid},
		{"backups", locales.WorldBackups(), locked},
		{"delete", locales.MoveToTrash(), locked},
	}
}

func (p *Instances) editWorld(file instancecontent.File, mode string, actions InstancesActions) {
	p.worldPath = ""
	if mode == "datapacks" {
		actions.WorldPacks(file)
		return
	}
	if mode == "backup" || mode == "backups" {
		actions.WorldArchive(mode, file)
		return
	}
	actions.EditWorld(file, mode)
}

func (p *Instances) worldImportMenu(c *ui.Context, data InstancesData, actions InstancesActions) {
	c.Key("world-import")
	holder := ui.Row(c).Shrink(0)
	value := ui.Local(holder, "world-import-mode", func() string { return "" })
	holder.Children(func() {
		menu := kit.SelectBase(c, value)
		kit.MenuButton(c, menu.Trigger, locales.Import(), menu.Open()).Label(locales.ImportWorld()).Disabled(data.Busy || data.InstanceBusy).OnChange(func() {
			mode := *value
			*value = ""
			actions.WorldArchive(mode, instancecontent.File{})
		})
		menu.Popup(func(panel ui.Element) {
			kit.PopupPanel(c, panel, 168).Role(ui.RoleMenu)
			for _, item := range []struct{ mode, label string }{{"file", locales.Archive()}, {"directory", locales.Folder()}, {"backups", locales.RestoreFromBackup()}} {
				c.Key(item.mode)
				kit.MenuItem(c, menu.Item(item.mode), item.label)
			}
		})
	})
}

func (p *Instances) worldMenu(c *ui.Context, file instancecontent.File, world instancecontent.World, data InstancesData, actions InstancesActions) {
	holder := ui.Row(c).Shrink(0)
	value := ui.Local(holder, "world-action", func() string { return "" })
	holder.Children(func() {
		menu := kit.SelectBase(c, value)
		kit.MenuButton(c, menu.Trigger, locales.Manage(), menu.Open()).Label(locales.WorldActions() + " · " + file.Name).OnChange(func() {
			mode := *value
			*value = ""
			if mode == "open" {
				actions.Open(file.Path)
			} else {
				p.editWorld(file, mode, actions)
			}
		})
		menu.Popup(func(panel ui.Element) {
			kit.PopupPanel(c, panel, 168).Role(ui.RoleMenu)
			c.Key("open")
			kit.MenuItem(c, menu.Item("open"), locales.OpenFolder())
			kit.Divider(c)
			for _, option := range worldEditOptions(world, data.Busy || data.InstanceBusy) {
				if option.mode == "delete" {
					kit.Divider(c)
				}
				c.Key(option.mode)
				kit.MenuItem(c, menu.Item(option.mode).Disabled(option.disabled), option.label)
			}
		})
	})
}
