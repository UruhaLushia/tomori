package kit

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

type ScrollTab struct {
	Label    string
	Icon     *ui.SVG
	Selected bool
	Count    int
	OnClick  func()
}

type ScrollTabsState struct {
	scroll  ui.ScrollState
	handles []ui.Handle
	reveal  int
	pending bool
	drag    scrollTabsDrag
}

type scrollTabsDrag struct {
	down, moved bool
	tab         int
	x, y, from  float32
}

func (s *ScrollTabsState) Reveal(index int) {
	s.reveal, s.pending = index, true
}

func ScrollTabs(c *ui.Context, state *ScrollTabsState, tabs []ScrollTab) {
	if len(state.handles) != len(tabs) {
		state.handles = make([]ui.Handle, len(tabs))
	}
	ui.ScrollHorizontal(c).FillWidth().MinWidth(0).Height(TabH()).Shrink(0).Gap(4).
		TrackScroll(&state.scroll).Scrollbars(false).HandleInput(func(ev ui.InputEvent) bool {
		return scrollTabsInput(c, state, tabs, ev)
	}).Children(func() {
		for i, tab := range tabs {
			c.Key(i)
			var el ui.Element
			if tab.Icon != nil {
				el = IconTab(c, tab.Icon, tab.Label, tab.Selected, tab.OnClick)
			} else {
				el = NavTab(c, tab.Label, tab.Selected, tab.OnClick)
			}
			el = el.Shrink(0).Bind(&state.handles[i])
			if tab.Selected && state.pending && state.reveal == i {
				el.ScrollIntoView()
			}
			if tab.Count > 0 {
				el.Label(fmt.Sprintf("%s %d", tab.Label, tab.Count)).Children(func() {
					Label(c, fmt.Sprint(tab.Count), Caption)
				})
			}
		}
	})
	state.pending = false
}

func scrollTabsInput(c *ui.Context, state *ScrollTabsState, tabs []ScrollTab, ev ui.InputEvent) bool {
	drag := &state.drag
	switch ev.Kind {
	case ui.InputScroll:
		if ev.DX != 0 || ev.DY == 0 || ev.Mods != 0 {
			return false
		}
		before := state.scroll.X
		state.scroll.X = max(0, min(state.scroll.MaxX, before+ev.DY))
		return state.scroll.X != before
	case ui.InputPointerDown:
		if ev.Button != 0 || state.scroll.MaxX <= 0 {
			return false
		}
		*drag = scrollTabsDrag{down: true, tab: scrollTabAt(c, state, ev.X, ev.Y), x: ev.X, y: ev.Y, from: state.scroll.X}
		return true
	case ui.InputPointerMove:
		if !drag.down {
			return false
		}
		dx, dy := ev.X-drag.x, ev.Y-drag.y
		if dx*dx+dy*dy > 36 {
			drag.moved = true
		}
		if drag.moved {
			if c.Direction() == ui.RightToLeft {
				dx = -dx
			}
			state.scroll.X = max(0, min(state.scroll.MaxX, drag.from-dx))
		}
		return true
	case ui.InputPointerUp:
		if ev.Button != 0 || !drag.down {
			return false
		}
		chosen := drag.tab
		click := !drag.moved && chosen >= 0 && state.scroll.FocusWithin(c) && scrollTabAt(c, state, ev.X, ev.Y) == chosen
		*drag = scrollTabsDrag{}
		if click && chosen < len(tabs) && tabs[chosen].OnClick != nil {
			tabs[chosen].OnClick()
		}
		return true
	}
	return false
}

func scrollTabAt(c *ui.Context, state *ScrollTabsState, x, y float32) int {
	strip := state.scroll.Bounds(c)
	point := ui.Point{X: strip.X + x, Y: strip.Y + y}
	if !strip.Contains(point.X, point.Y) {
		return -1
	}
	for i, tab := range state.handles {
		if tab.Bounds(c).Contains(point.X, point.Y) {
			return i
		}
	}
	return -1
}
