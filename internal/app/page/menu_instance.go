package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Instances) instanceMenu(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	p.instanceMenuTrigger(c, v, data, actions, false)
}

func (p *Instances) instanceIconMenu(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	p.instanceMenuTrigger(c, v, data, actions, true)
}

func (p *Instances) instanceMenuTrigger(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions, icon bool) {
	c.Key("instance-menu:" + v.Key())
	var trigger ui.Element
	if icon {
		trigger = kit.IconButton(c, kit.More, false, nil).Label(locales.MoreInstanceActions()).Tooltip(locales.Manage())
	} else {
		trigger = kit.Button(c, locales.Manage(), nil).Label(locales.MoreInstanceActions())
	}
	trigger.Role(ui.RoleMenuButton)
	kit.ContextMenuTrigger(trigger, trigger)
	p.instanceContextMenu(c, trigger, v, data, actions)
}

type instanceOption struct {
	value, label string
	disabled     bool
	separator    bool
}

func instanceOptions(v minecraft.Instance, data InstancesData) []instanceOption {
	busy := data.InstanceBusy
	if data.InUse != nil {
		busy = data.InUse(v.Key())
	}
	locked := data.Busy || busy || !v.CanLaunch()
	options := []instanceOption{
		{value: "open", label: locales.OpenFolder(), disabled: instanceRoot(v, data) == ""},
		{value: "rename", label: locales.Rename(), disabled: locked, separator: true},
		{value: "icon", label: locales.ChangeIcon(), disabled: locked},
		{value: "settings", label: locales.InstanceSettings(), disabled: data.Busy || busy},
		{value: "components", label: locales.InstanceComponents(), disabled: locked},
		{value: "copy", label: locales.CopyInstance(), disabled: locked},
		{value: "refresh", label: locales.RefreshContent(), disabled: data.Content.Loading, separator: true},
		{value: "repair", label: locales.VerifyAndRepair(), disabled: locked},
	}
	if v.PackFormat != "" || v.Format == minecraft.InstancePrism || v.PackSource.Provider != "" {
		options = append(options,
			instanceOption{value: "update-pack", label: locales.UpdateModpack(), disabled: locked, separator: true},
			instanceOption{value: "export", label: locales.ExportModpack(), disabled: locked},
			instanceOption{value: "restore-pack", label: locales.RestoreModpack(), disabled: locked},
			instanceOption{value: "associate-pack", label: locales.LinkModpack(), disabled: locked || v.PackSource.Provider != ""})
	}
	return append(options,
		instanceOption{value: "link", label: locales.CopyLaunchLink(), disabled: !v.CanLaunch(), separator: true},
		instanceOption{value: "shortcut", label: locales.CreateShortcut(), disabled: data.Busy || !v.CanLaunch()},
		instanceOption{value: "delete", label: locales.DeleteInstance(), disabled: locked, separator: true})
}

func instanceRoot(v minecraft.Instance, data InstancesData) string {
	if data.ContentRoot != nil {
		return data.ContentRoot(v)
	}
	if data.Content.Key == v.Key() {
		return data.Content.Root
	}
	return v.GameDir
}

func runInstanceAction(action string, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	switch action {
	case "settings":
		actions.Settings(v.Key())
	case "refresh":
		if data.Resources != nil && data.Resources.Open && data.Resources.Embedded {
			if actions.ResourceActions.Refresh != nil {
				actions.ResourceActions.Refresh()
			}
		} else if actions.RefreshContent != nil {
			actions.RefreshContent()
		}
	case "open":
		actions.Open(instanceRoot(v, data))
	case "repair":
		actions.Repair(v.Key())
	case "rename", "icon", "delete":
		actions.Edit(v.Key(), action)
	case "copy":
		actions.Copy(v.Key())
	case "shortcut":
		actions.Shortcut(v.Key())
	case "link":
		actions.CopyLink(v.Key())
	case "export":
		actions.Export(v.Key())
	case "update-pack":
		actions.UpdatePack(v.Key())
	case "associate-pack":
		actions.AssociatePack(v.Key())
	case "restore-pack":
		actions.RestorePack(v.Key())
	case "components":
		actions.Components(v.Key())
	}
}

func (p *Instances) instanceContextMenu(c *ui.Context, row ui.Element, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	InstanceContextMenu(c, row, v, data, actions, nil)
}

func InstanceContextMenu(c *ui.Context, row ui.Element, v minecraft.Instance, data InstancesData, actions InstancesActions, details func()) {
	kit.ContextMenu(c, row, func(menu *kit.ContextActions) {
		if details != nil {
			menu.Item(c, "details", locales.InstanceDetails(), false, details)
		}
		for _, option := range instanceOptions(v, data) {
			tone := kit.Plain
			if option.separator {
				menu.Separator(c)
			}
			if option.value == "delete" {
				tone = kit.DangerTone
			}
			menu.Item(c, option.value, option.label, option.disabled, func() { runInstanceAction(option.value, v, data, actions) }, tone)
		}
	})
}
