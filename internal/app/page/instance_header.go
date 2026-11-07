package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Instances) instanceHeader(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) {
	if actions.LoadIcon != nil {
		actions.LoadIcon(v)
	}
	compact := kit.Compact(c)
	ui.Column(c).FillWidth().Shrink(0).Gap(kit.Gap(c)).Children(func() {
		if !p.inline {
			kit.BackBar(c, locales.Library(), func() { p.leaveDetail(actions) }, nil)
		}
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
			iconSize := float32(48)
			if compact {
				iconSize = 40
			}
			kit.InstanceIcon(c, data.Icons[v.Key()], iconSize)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				kit.Line(c, v.Name, kit.Heading).ContentTransition(kit.ValueMotion)
				instanceStatus(c, v, data)
			})
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
				if !compact {
					kit.IconButton(c, kit.Folder, false, func() { actions.Open(instanceRoot(v, data)) }).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
					kit.IconButton(c, kit.Sliders, false, func() { actions.Settings(v.Key()) }).Label(locales.InstanceSettings()).Tooltip(locales.InstanceSettings())
				}
				p.instanceIconMenu(c, v, data, actions)
				instanceLaunchButton(c, v, data, actions)
			})
		})
		p.contentTabs(c, data, actions)
	})
}

func instanceActivity(v minecraft.Instance, data InstancesData) InstanceActivity {
	if data.Activity == nil {
		return InstanceActivity{}
	}
	return data.Activity(v.Key())
}

func instanceStatus(c *ui.Context, v minecraft.Instance, data InstancesData) {
	activity := instanceActivity(v, data)
	if !activity.Active && !activity.Running && !activity.Finishing && !activity.Canceling {
		return
	}
	text, color, spinner, dot := "", kit.Muted, false, false
	switch {
	case activity.Canceling:
		text, spinner = locales.Stopping(), true
	case activity.Finishing:
		text, spinner = locales.RunningPostExitCommands(), true
	case activity.Running:
		text, color, dot = locales.Running3(), kit.Green, true
	case activity.Active:
		text, spinner = activity.Status, true
		if text == "" {
			text = locales.Launching()
		}
	}
	ui.Row(c).FillWidth().MinWidth(0).Height(kit.LinePx(kit.Caption)).Gap(6).AlignItems(ui.Center).Children(func() {
		if spinner {
			c.Key("status-spinner")
			ui.Spinner(c).Size(12, 12).TextColor(kit.Accent)
		} else if dot {
			kit.StatusDot(c, color)
		}
		kit.Line(c, text, kit.Caption).Shrink(1).ContentTransition(kit.ValueMotion)
	})
}

func instanceLaunchButton(c *ui.Context, v minecraft.Instance, data InstancesData, actions InstancesActions) ui.Element {
	key := v.Key()
	launch := func() {
		if actions.Launch != nil {
			actions.Launch(key)
		}
	}
	stop := func() {
		if actions.Stop != nil {
			actions.Stop(key)
		}
	}
	busy := data.InstanceBusy
	if data.InUse != nil {
		busy = data.InUse(key)
	}
	activity := instanceActivity(v, data)
	label, icon, face, click, disabled := locales.LaunchGame(), kit.Play, kit.LaunchPrimary, launch, data.Busy || busy || !v.CanLaunch()
	switch {
	case activity.Canceling:
		label, icon, face, click, disabled = locales.StopGame(), kit.Stop, kit.LaunchDanger, stop, true
	case activity.Finishing:
		label, icon, face, click, disabled = locales.CancelCommand(), kit.Close, kit.LaunchSecondary, stop, false
	case activity.Running:
		label, icon, face, click, disabled = locales.StopGame(), kit.Stop, kit.LaunchDanger, stop, false
	case activity.Active:
		label, icon, face, click, disabled = locales.CancelLaunch(), kit.Close, kit.LaunchSecondary, stop, false
	}
	button := kit.LaunchButton(c, face, icon, label, click).Disabled(disabled)
	if kit.Compact(c) {
		button.Width(kit.W(128))
	}
	return button
}
