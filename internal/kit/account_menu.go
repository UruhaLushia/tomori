package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/locales"
)

type AccountItem struct {
	ID, Name, Kind string
	Avatar         *ui.Bitmap
	Initial        string
	Current        bool
}

type accountMenuState struct {
	open, restore, placed, focus bool
	anchor                       ui.Rect
	w, h                         float32
	trigger                      ui.Handle
}

func AccountMenu(c *ui.Context, trigger ui.Element, items []AccountItem, onSelect func(string), onAdd, onManage func()) {
	s := ui.Local(trigger, "account-menu", func() accountMenuState { return accountMenuState{} })
	trigger.Bind(&s.trigger).Role(ui.RoleMenuButton).Expanded(s.open).OnClick(func() {
		s.open, s.placed = !s.open, false
		s.focus = s.open
		if s.open {
			trigger.Focus()
		}
	})
	if s.restore {
		trigger.Focus()
		s.restore = false
	}
	w, h := c.Size()
	anchor := s.trigger.Bounds(c)
	if s.open && s.placed && (s.w != w || s.h != h || s.anchor != anchor) {
		s.open = false
	}
	if !s.open {
		s.placed = false
		return
	}
	opening := !s.placed
	s.w, s.h, s.anchor, s.placed = w, h, anchor, true
	trigger.Background(Press)
	closeMenu := func(run func()) {
		s.open, s.restore = false, true
		if run != nil {
			run()
		}
	}
	ui.PopoverBase(c, trigger, &s.open, func(panel ui.Element) {
		contentHeight := float32(len(items))*H(40, Strong, Caption) + 2*ControlH() + 8
		if len(items) > 0 {
			contentHeight += 9
		}
		placePopup(c, panel, trigger, max(anchor.W, W(240)), contentHeight)
		panel.Role(ui.RoleMenu).FocusGroup(ui.Vertical)
		ui.Column(c).Fill().Children(func() {
			if len(items) > 0 {
				ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Children(func() {
						current := false
						for _, item := range items {
							current = current || item.Current
						}
						for i, item := range items {
							c.Key(item.ID)
							row := RowButton(c, item.Name, item.Current, func() {
								closeMenu(func() {
									if !item.Current && onSelect != nil {
										onSelect(item.ID)
									}
								})
							}).Role(ui.RoleMenuItem).FillWidth().Height(H(40, Strong, Caption)).Padding(0, 8).Gap(8).Radius(RadiusM)
							if item.Current || !current && i == 0 {
								if s.focus && !opening {
									row.Focus().ScrollIntoView()
								}
							}
							row.Children(func() {
								PlayerAvatar(c, item.Avatar, item.Initial, 28)
								ui.Column(c).Grow(1).MinWidth(0).Children(func() {
									color := Text
									if item.Current {
										color = AccentText
									}
									Line(c, item.Name, Strong).TextColor(color)
									Line(c, item.Kind, Caption)
								})
								ui.Box(c).Size(16, 16).Shrink(0).Center().Children(func() {
									if item.Current {
										ui.Icon(c, Check).FontSize(16).TextColor(AccentText)
									}
								})
							})
						}
					})
				})
				Divider(c).Margin(4, 8).Shrink(0)
			}
			for i, action := range []struct {
				label string
				icon  *ui.SVG
				run   func()
			}{{locales.AddAccount(), Plus, onAdd}, {locales.ManageAccounts(), Sliders, onManage}} {
				c.Key(action.label)
				row := RowButton(c, action.label, false, func() { closeMenu(action.run) }).Role(ui.RoleMenuItem).FillWidth().Height(ControlH()).Padding(0, 8).Radius(RadiusM).Justify(ui.Start).Gap(8).Disabled(action.run == nil)
				if len(items) == 0 && i == 0 {
					if s.focus && !opening {
						row.Focus()
					}
				}
				row.Shrink(0).Children(func() {
					ui.Icon(c, action.icon).FontSize(16).TextColor(Muted)
					Line(c, action.label, Body)
				})
			}
		})
	})
	if !opening {
		s.focus = false
	}
}
