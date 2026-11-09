package page

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Instances) serverMembersDialog(c *ui.Context, data *ServersData) {
	if !p.serverMembersOpen || p.serverMembers < 0 || p.serverMembers >= len(data.List.Entries) {
		p.serverMembersOpen = false
		return
	}
	entry := data.List.Entries[p.serverMembers]
	result := data.Status[p.serverMembers]
	if !result.Done || result.Error != "" {
		p.serverMembersOpen = false
		return
	}
	kit.Dialog(c, &p.serverMembersOpen, kit.DialogM, func(_, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, entry.Name, func() { p.serverMembersOpen = false })
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.ProjectIcon(c, result.Icon, 44)
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					if result.Value.Description != "" {
						kit.Line(c, result.Value.Description, kit.Body).MaxLines(2).Ellipsis("…")
					}
					kit.Line(c, result.Value.Players(), kit.Caption).TextColor(kit.Green).Shrink(0)
				})
			})
			kit.Divider(c)
			kit.Section(c, locales.Online(), nil)
			if len(result.Value.Members) == 0 {
				kit.EmptyState(c, kit.User, locales.Online(), result.Value.Players(), nil).MinHeight(96)
				return
			}
			ui.Scroll(c).FillWidth().Height(min(360, float32(len(result.Value.Members))*kit.RowDoubleH())).Scrollbars(false).Children(func() {
				ui.Column(c).FillWidth().Shrink(0).Padding(0, kit.ScrollbarGutter).Gap(2).Children(func() {
					for _, member := range result.Value.Members {
						ui.Row(c).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).AlignItems(ui.Center).Children(func() {
							id := member.UUID
							if id == "" {
								id = member.Name
							}
							kit.PlayerAvatar(c, result.MemberIcons[id], member.Name, 28)
							kit.Line(c, member.Name, kit.Strong).Grow(1)
						})
					}
				})
			})
		})
	})
}
