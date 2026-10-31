package kit

import "github.com/egoist/mygo/ui"

type contextMenuState struct {
	open, restore, placed bool
	x, y, w, h            float32
	anchor                ui.Rect
}

type ContextActions struct {
	state            *contextMenuState
	first            bool
	rows, separators int
}

func MoreMenu(c *ui.Context, label string, build func(*ContextActions)) ui.Element {
	el := ui.Box(c).Size(28, 28).Shrink(0)
	el.Children(func() { ContextMenuButton(c, el, label) })
	ContextMenu(c, el, build)
	return el
}

// ContextMenuButton opens the row's existing context menu, so pointer and
// keyboard entry points always expose the same actions.
func ContextMenuButton(c *ui.Context, target ui.Element, label string) ui.Element {
	button := IconButton(c, More, false, nil).Size(28, 28).Label(label).Tooltip(label)
	ContextMenuTrigger(target, button)
	return button
}

func ContextMenuTrigger(target, trigger ui.Element) {
	state := ui.Local(target, "context-menu", func() contextMenuState { return contextMenuState{} })
	trigger.OnClick(func() {
		row, button := target.Bounds(), trigger.Bounds()
		state.x, state.y, state.open, state.placed = button.X-row.X, button.Y+button.H-row.Y, true, false
	})
}

func ContextMenu(c *ui.Context, target ui.Element, build func(*ContextActions)) {
	state := ui.Local(target, "context-menu", func() contextMenuState { return contextMenuState{} })
	if state.restore {
		target.Focus()
		state.restore = false
	}
	target.ContextMenu(func(*ui.Menu) {
		target.Focus()
		state.x, state.y, _ = target.PointerPosition()
		state.open, state.placed = true, false
	})
	if target.Shortcut(ui.Shift, ui.KeyF10) || target.Shortcut(0, ui.KeyContextMenu) {
		state.x, state.y, state.open, state.placed = 0, 24, true, false
	}
	if !state.open {
		return
	}
	width, height := c.Size()
	// Press animations change visual bounds without moving the menu's layout.
	bounds := target.LayoutBounds()
	if state.placed && (state.w != width || state.h != height || state.anchor != bounds) {
		state.open, state.placed = false, false
		return
	}
	state.w, state.h, state.anchor, state.placed = width, height, bounds, true
	target.Children(func() {
		c.Key("context-menu-anchor")
		anchor := ui.Box(c).Absolute().Left(state.x).Top(state.y).Size(1, 1)
		ui.Overlay(c, func() {
			c.Key("context-menu-backdrop")
			back := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).OnClick(func() { state.open = false })
			if back.RightClicked() {
				state.open = false
			}
			c.Key("context-menu-panel")
			panel := ui.Column(c).AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).OnClick(func() {})
			panel.Role(ui.RoleMenu).FocusGroup(ui.Vertical)
			menu := &ContextActions{state: state, first: true}
			panel.Children(func() {
				if panel.OverlayShortcut(0, ui.KeyEscape) {
					state.open, state.restore = false, true
				}
				ui.Scroll(c).Scrollbars(false).FillWidth().Grow(1).MinHeight(0).Children(func() {
					build(menu)
				})
			})
			placePopup(c, panel, anchor, W(184), float32(menu.rows)*ControlH()+float32(menu.separators*9+8))
		})
	})
}

func (m *ContextActions) Item(c *ui.Context, key, label string, disabled bool, run func(), tones ...Tone) ui.Element {
	m.rows++
	c.Key(key)
	state := m.state
	item := ui.ButtonBase(c).Disabled(disabled || run == nil).OnClick(func() {
		state.open, state.restore = false, true
		if run != nil {
			run()
		}
	})
	if m.first && !item.IsDisabled() {
		item.AutoFocus()
		m.first = false
	}
	return MenuItem(c, item, label, tones...)
}

func (m *ContextActions) Separator(c *ui.Context) {
	m.separators++
	ui.Box(c).AlignSelf(ui.Stretch).Height(1).Margin(4, 8).Background(LineSoft)
}
