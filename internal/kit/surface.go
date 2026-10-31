package kit

import "github.com/egoist/mygo/ui"

func Inset(c *ui.Context, el ui.Element) ui.Element {
	return el.Radius(RadiusM).Background(InsetColor).Clip()
}

func Floating(c *ui.Context, el ui.Element) ui.Element {
	return Card(c, el).Shadow(0, 4, 14, 0, Shadow)
}

func RowButton(c *ui.Context, label string, selected bool, onClick func()) ui.Element {
	el := ui.ButtonBase(c).FillWidth().Radius(RadiusS).AlignItems(ui.Center).Justify(ui.Start).OnClick(onClick)
	el.Label(label)
	background := ui.Transparent
	if selected {
		background = AccentSoft
	} else if el.Pressed() {
		background = Press
	} else if el.Hovered() || el.FocusVisible() {
		background = Hover
	}
	return LayoutMotion(el).Background(el.AnimateColor("bg", background, MotionFast))
}

func IconBadge(c *ui.Context, icon *ui.SVG, size float32, colors ...ui.Color) ui.Element {
	radius, fontSize := RadiusS, float32(16)
	if size > 32 {
		radius, fontSize = RadiusM, 20
	}
	color := Muted
	if len(colors) > 0 {
		color = colors[0]
	}
	return ui.Box(c).Size(size, size).Shrink(0).Radius(radius).Center().Background(InsetColor).Children(func() {
		ui.Icon(c, icon).FontSize(IconPx(fontSize)).TextColor(color)
	})
}

func Thumb(c *ui.Context, label string, size float32, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Size(size, size).Shrink(0).Radius(RadiusM).Clip().OnClick(onClick).Label(label)
	return LayoutMotion(CardMotion(el))
}

// StatBlock is the compact, icon-free statistic used by the instance overview.
// It intentionally has no fixed height: the token padding and text line heights
// determine the block size while keeping all four blocks aligned.
func StatBlock(c *ui.Context, label, value string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Column().AlignItems(ui.Start).Grow(1).Basis(W(80)).MinWidth(W(80)).Shrink(0).Padding(12).Gap(4).Radius(RadiusM).
		Background(InsetColor).OnClick(onClick).Label(label)
	if el.Hovered() || el.FocusVisible() {
		el.Background(Hover)
	}
	return LayoutMotion(el).Children(func() {
		Line(c, label, Caption)
		Line(c, value, Strong).TextColor(Text).ContentTransition(NumberMotion)
	})
}

func Property(c *ui.Context, label, value string) ui.Element {
	return ui.Row(c).FillWidth().Height(ControlH()).Gap(12).AlignItems(ui.Center).Children(func() {
		Line(c, label, Caption).Width(W(72)).Shrink(0)
		Line(c, MiddleEllipsis(value, 48), Body).Grow(1).Basis(0).MinWidth(0).Selectable().Tooltip(value)
	})
}

func EmptyState(c *ui.Context, icon *ui.SVG, title, hint string, actions func()) ui.Element {
	return PresenceMotion(ui.Column(c.Key(motionStateKey("empty"))).Grow(1).FillWidth().AlignItems(ui.Center).Justify(ui.Center)).Children(func() {
		if icon != nil {
			IconBadge(c, icon, 40).Margin(0, 0, 12, 0)
		}
		Label(c, title, Strong)
		if hint != "" {
			Label(c, hint, Caption).Margin(4, 0, 0, 0).MaxWidth(W(320)).Wrap().TextAlign(ui.Center)
		}
		if actions != nil {
			ui.Row(c).Margin(16, 0, 0, 0).Gap(8).Children(actions)
		}
	})
}

func Loading(c *ui.Context) ui.Element {
	return PresenceMotion(ui.Box(c.Key(motionStateKey("loading"))).Grow(1).FillWidth().Center()).Children(func() { ui.Spinner(c).Size(16, 16) })
}

func LogLevel(c *ui.Context, level string, color ui.Color) ui.Element {
	return Line(c, level, Mono).FontWeight(weight(Strong)).TextColor(color)
}

func ListRowBackground(c *ui.Context, el ui.Element, selected bool) ui.Element {
	target := ui.Transparent
	if selected {
		target = AccentSoft
	} else if el.Pressed() {
		target = Press
	} else if el.Hovered() || el.FocusVisible() {
		target = Hover
	}
	return el.Background(el.AnimateColor("row-bg", target, hoverAnim))
}

func StatusDot(c *ui.Context, color ui.Color) ui.Element {
	el := ui.Box(c).Size(8, 8).Radius(RadiusFull)
	return el.Background(el.AnimateColor("status", color, MotionFast))
}

func MetaLine(parts ...string) string {
	result := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if result != "" {
			result += " · "
		}
		result += part
	}
	return result
}
