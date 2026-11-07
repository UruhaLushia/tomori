package kit

import "github.com/egoist/mygo/ui"

var (
	selectAnim        = MotionMove
	topNavSelectAnim  = MotionFast
	sidebarSelectAnim = MotionInOut
	sidebarRailAnim   = MotionNavRail
)

func Tabs(c *ui.Context, selected *int, labels []string, change func()) ui.Element {
	tabs := ui.TabsBase(c, selected, len(labels))
	return tabs.List.Wrap().Gap(4).OnChange(change).Children(func() {
		for i, label := range labels {
			tab := tabs.Tab(i).Height(TabH()).Shrink(0).Padding(0, 8).Radius(RadiusS)
			color := Muted
			if i == *selected {
				color = AccentText
			} else if tab.Hovered() {
				color = Text
			}
			tab.Children(func() {
				if i == *selected {
					tab.Background(tab.AnimateColor("bg", AccentSoft, selectAnim))
				} else {
					tab.Background(tab.AnimateColor("bg", ui.Transparent, selectAnim))
				}
				style := Body
				if i == *selected {
					style = Strong
				}
				Label(c, label, style).TextColor(tab.AnimateColor("fg", color, MotionFast)).SingleLine().Shrink(0).ContentTransition(ValueMotion)
			})
		}
	})
}

func NavRow(c *ui.Context, icon *ui.SVG, label string, selected bool, onClick func()) ui.Element {
	el := RowButton(c, label, selected, onClick).Height(ControlH()).Padding(0, 8).Gap(8).Radius(RadiusS)
	return el.Children(func() {
		if icon != nil {
			color := Muted
			if selected {
				color = AccentText
			}
			ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(el.AnimateColor("nav-icon-fg", color, sidebarSelectAnim))
		}
		navigationLabel(c, el, label, selected)
	})
}

func NavSubRow(c *ui.Context, label string, selected bool, onClick func()) ui.Element {
	el := RowButton(c, label, false, onClick).Height(TabH()).Padding(0, 8).Radius(RadiusS)
	return el.Children(func() {
		style, color := Body, Muted
		if selected {
			style, color = Strong, AccentText
		}
		Label(c, label, style).SingleLine().Shrink(0).TextColor(el.AnimateColor("nav-fg", color, sidebarSelectAnim))
	})
}

// NavSubGroup owns the continuous rail so its selected segment moves between
// rows without recreating or fading it (L-SHELL-4).
func NavSubGroup(c *ui.Context, selected int, rows func()) ui.Element {
	return ui.Column(c).FillWidth().Gap(4).Padding(0, 0, 0, 24).Children(func() {
		ui.Box(c).Absolute().Left(16).Top(2).Bottom(2).Width(1).Background(LineColor).PassThrough()
		if selected >= 0 {
			ui.Box(c.Key("sub-selection")).Absolute().Left(15.5).Top(0).Width(2).Height(16).Radius(RadiusFull).Background(Accent).PassThrough().
				AnimatePose(ui.Pose{Y: float32(selected)*(TabH()+4) + (TabH()-16)/2}, LayoutAnimation(sidebarRailAnim))
		}
		rows()
	})
}

func navigationLabel(c *ui.Context, row ui.Element, label string, selected bool) {
	color := Text
	if selected {
		color = AccentText
	}
	style := Body
	if selected {
		style = Strong
	}
	Line(c, label, style).Grow(1).TextColor(row.AnimateColor("nav-fg", color, sidebarSelectAnim))
}

func SideHeading(c *ui.Context, label string) ui.Element {
	return Label(c, label, SectionText).Padding(8, 8, 4, 8).TextColor(Muted)
}

func NavPill(c *ui.Context, icon *ui.SVG, label string, selected bool, onClick func()) ui.Element {
	if icon == nil {
		return NavTab(c, label, selected, onClick)
	}
	el := ui.ButtonBase(c).Height(TabH()).Gap(8).Padding(0, 8).Radius(RadiusS).AlignItems(ui.Center).OnClick(onClick).Label(label)
	background := ui.Transparent
	if selected {
		background = AccentSoft
	} else if el.Hovered() || el.FocusVisible() {
		background = Hover
	}
	return el.Children(func() {
		if selected {
			ui.Box(c.Key("top-nav-selection")).Absolute().Fill().Radius(RadiusS).Background(AccentSoft).PassThrough().
				Transition(ui.ElementTransition{Position: true, Size: true, Colors: true, Animation: &MotionNavRail, Match: "top-nav-selection"})
		} else {
			el.Background(el.AnimateColor("bg", background, topNavSelectAnim))
		}
		color := selectedColor(selected)
		ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(el.AnimateColor("fg", color, topNavSelectAnim))
		style := Body
		if selected {
			style = Strong
		}
		Label(c, label, style).SingleLine().Shrink(0).TextColor(el.AnimateColor("fg", color, topNavSelectAnim))
	})
}

func NavTab(c *ui.Context, label string, selected bool, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Height(TabH()).Padding(0, 8).Radius(RadiusS).AlignItems(ui.Center).OnClick(onClick).Label(label)
	background := ui.Transparent
	if selected {
		background = AccentSoft
	} else if el.Hovered() || el.FocusVisible() {
		background = Hover
	}
	el.Background(el.AnimateColor("bg", background, topNavSelectAnim))
	return el.Children(func() {
		color := Muted
		if selected {
			color = AccentText
		}
		style := Body
		if selected {
			style = Strong
		}
		Label(c, label, style).SingleLine().Shrink(0).TextColor(el.AnimateColor("fg", color, topNavSelectAnim))
	})
}

func IconTab(c *ui.Context, icon *ui.SVG, label string, selected bool, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Height(TabH()).Padding(0, 10, 0, 8).Gap(6).Radius(RadiusS).AlignItems(ui.Center).OnClick(onClick).Label(label)
	background := ui.Transparent
	if selected {
		background = AccentSoft
	} else if el.Hovered() || el.FocusVisible() {
		background = Hover
	}
	el.Background(el.AnimateColor("bg", background, topNavSelectAnim))
	return el.Children(func() {
		color := Muted
		if selected {
			color = AccentText
		} else if el.Hovered() {
			color = Text
		}
		ink := el.AnimateColor("fg", color, topNavSelectAnim)
		ui.Icon(c, icon).FontSize(IconPx(14)).TextColor(ink)
		style := Body
		if selected {
			style = Strong
		}
		Label(c, label, style).SingleLine().Shrink(0).TextColor(ink)
	})
}

func selectedColor(selected bool) ui.Color {
	if selected {
		return AccentText
	}
	return Muted
}
