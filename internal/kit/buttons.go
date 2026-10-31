package kit

import "github.com/egoist/mygo/ui"

var hoverAnim = MotionFast

func IconButton(c *ui.Context, icon *ui.SVG, active bool, onClick func()) ui.Element {
	return iconButton(c, icon, active, onClick, MotionFast)
}

func SideNavIconButton(c *ui.Context, icon *ui.SVG, active bool, onClick func()) ui.Element {
	return iconButton(c, icon, active, onClick, MotionInOut)
}

func iconButton(c *ui.Context, icon *ui.SVG, active bool, onClick func(), animation ui.Animation) ui.Element {
	el := ui.ButtonBase(c).Size(32, 32).Shrink(0).Radius(RadiusS).Center().OnClick(onClick)
	target := ui.Transparent
	switch {
	case active:
		target = AccentSoft
	case el.Hovered():
		target = Hover
	}
	ControlMotion(el).Background(el.AnimateColor("bg", target, animation))
	return el.Children(func() {
		color := Muted
		if active {
			color = AccentText
		} else if el.Hovered() {
			color = Text
		}
		ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(el.AnimateColor("fg", color, animation)).ContentTransition(ImageMotion)
	})
}

func WindowButton(c *ui.Context, icon *ui.SVG, danger bool, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(RadiusS).Center()
	el.OnClick(onClick)
	hovered := el.Hovered()
	background := ui.Transparent
	switch {
	case hovered && danger:
		background = Red
	case hovered:
		background = Hover
	}
	el.Background(el.AnimateColor("bg", background, MotionFast))
	return el.Children(func() {
		color := Muted
		if hovered && danger {
			color = accentInk(Red)
		}
		ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(color)
	})
}

func Button(c *ui.Context, label string, onClick func()) ui.Element {
	return labelButton(c, label, false, onClick)
}

func Primary(c *ui.Context, label string, onClick func()) ui.Element {
	return labelButton(c, label, true, onClick)
}

func Danger(c *ui.Context, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Height(ControlH()).Padding(0, 12).Radius(RadiusS).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).OnClick(onClick).Label(label)
	bg := RedSoft
	if el.Pressed() {
		bg = Press
	} else if el.Hovered() || el.FocusVisible() {
		bg = Hover
	}
	return ControlMotion(el).Background(el.AnimateColor("bg", bg, MotionFast)).Children(func() { Label(c, label, Strong).TextColor(Red).SingleLine().Shrink(0) })
}

func labelButton(c *ui.Context, label string, primary bool, onClick func()) ui.Element {
	el := buttonFace(c, ui.ButtonBase(c), primary, false).Shrink(0).Justify(ui.Center).Label(label).OnClick(onClick)
	return el.Children(func() {
		style := Body
		if primary {
			style = Strong
		}
		Label(c, label, style).SingleLine().Shrink(0).ContentTransition(ValueMotion)
	})
}

func TextButton(c *ui.Context, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Shrink(0).Height(TabH()).Padding(0, 8).Radius(RadiusS).OnClick(onClick)
	color := Muted
	if el.Hovered() || el.Pressed() {
		color = AccentText
	}
	return ControlMotion(el).Children(func() {
		Label(c, label, Body).TextColor(el.AnimateColor("fg", color, MotionFast)).SingleLine().ContentTransition(ValueMotion)
	})
}

func BackButton(c *ui.Context, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Shrink(0).Height(ControlH()).Padding(0, 8).Gap(8).Radius(RadiusS).AlignItems(ui.Center).Label(label).OnClick(onClick)
	color, background := Text, ui.Transparent
	if el.Hovered() || el.Pressed() {
		color, background = Text, Hover
	}
	el.Background(el.AnimateColor("bg", background, hoverAnim))
	return ControlMotion(el).Children(func() {
		ui.Icon(c, ArrowLeft).FontSize(IconPx(16)).TextColor(color)
		Label(c, label, Body).TextColor(color).SingleLine()
	})
}

func Ghost(c *ui.Context, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Shrink(0).Height(ControlH()).Padding(0, 8).Radius(RadiusS).OnClick(onClick).Label(label)
	background := ui.Transparent
	if el.Hovered() || el.Pressed() {
		background = Hover
	}
	el.Background(el.AnimateColor("bg", background, MotionFast))
	return ControlMotion(el).Children(func() {
		Label(c, label, Body).SingleLine().Shrink(0).ContentTransition(ValueMotion)
	})
}

func IconLabel(c *ui.Context, icon *ui.SVG, label string, onClick func()) ui.Element {
	el := buttonFace(c, ui.ButtonBase(c), false, false).Shrink(0).OnClick(onClick).Label(label)
	return el.Children(func() { ui.Icon(c, icon).FontSize(IconPx(16)); Label(c, label, Body) })
}

func HeroButton(c *ui.Context, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Shrink(0).Height(H(40, Strong)).Padding(0, 12).Gap(8).Radius(RadiusM).TextColor(AccentInk).OnClick(onClick)
	background := Accent
	if el.Hovered() {
		background = AccentHi
	}
	return ControlMotion(el).Background(el.AnimateColor("bg", background, MotionFast)).Label(label)
}

func SelectTrigger(c *ui.Context, el ui.Element, open bool) ui.Element {
	return buttonFace(c, el, false, open)
}

func MenuButton(c *ui.Context, el ui.Element, label string, open bool) ui.Element {
	return menuButton(c, el, label, false, open)
}

func PrimaryMenuButton(c *ui.Context, el ui.Element, label string, open bool) ui.Element {
	return menuButton(c, el, label, true, open)
}

func menuButton(c *ui.Context, el ui.Element, label string, primary, open bool) ui.Element {
	el = buttonFace(c, el, primary, open).Shrink(0)
	return el.Role(ui.RoleMenuButton).Label(label).Value("").Children(func() {
		style := Body
		if primary {
			style = Strong
		}
		Label(c, label, style).SingleLine().Shrink(0).ContentTransition(ValueMotion)
	})
}

type LaunchFace int

const (
	LaunchPrimary LaunchFace = iota
	LaunchSecondary
	LaunchDanger
)

func LaunchButton(c *ui.Context, face LaunchFace, icon *ui.SVG, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Width(W(160)).Height(H(40, Strong)).Shrink(0).Padding(0, 12).Gap(8).Radius(RadiusM).Center().OnClick(onClick).Label(label)
	bg, ink := Accent, AccentInk
	switch face {
	case LaunchSecondary:
		bg, ink = Surface, Text
		el.Border(1, LineColor)
	case LaunchDanger:
		bg, ink = RedSoft, Red
	}
	if el.Pressed() {
		bg = Press
	} else if el.Hovered() || el.FocusVisible() {
		if face == LaunchPrimary {
			bg = AccentHi
		} else {
			bg = Hover
		}
	}
	return el.Background(el.AnimateColor("launch-bg", bg, MotionFast)).Children(func() {
		ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(ink).ContentTransition(ImageMotion)
		Label(c, label, Strong).SingleLine().Shrink(0).TextColor(ink).ContentTransition(ValueMotion)
	})
}

func QuickAction(c *ui.Context, running bool, label string, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(RadiusFull).Center().Label(label).Tooltip(label).OnClick(onClick)
	bg, ink, icon := AccentSoft, AccentText, Play
	if running {
		bg, ink, icon = RedSoft, Red, Stop
	}
	if el.Pressed() {
		bg = Press
	} else if el.Hovered() || el.FocusVisible() {
		bg = Hover
	}
	return ControlMotion(el).Background(el.AnimateColor("bg", bg, MotionFast)).Children(func() { ui.Icon(c, icon).FontSize(IconPx(12)).TextColor(ink) })
}

func ActionButton(c *ui.Context, face LaunchFace, icon *ui.SVG, label string, click func()) ui.Element {
	el := ui.ButtonBase(c).Height(ControlH()).Shrink(0).Padding(0, 12).Gap(8).Radius(RadiusS).AlignItems(ui.Center).Justify(ui.Center).OnClick(click).Label(label)
	bg, ink := Surface, Text
	switch face {
	case LaunchPrimary:
		bg, ink = Accent, AccentInk
	case LaunchDanger:
		bg, ink = RedSoft, Red
	default:
		el.Border(1, LineColor)
	}
	if el.Hovered() || el.FocusVisible() {
		if face == LaunchPrimary {
			bg = AccentHi
		} else {
			bg = Hover
		}
	}
	if el.Pressed() {
		bg = Press
	}
	return ControlMotion(el).Background(el.AnimateColor("bg", bg, MotionFast)).Children(func() {
		ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(ink)
		Label(c, label, Strong).TextColor(ink).SingleLine().Shrink(0)
	})
}

func buttonFace(c *ui.Context, el ui.Element, primary, active bool) ui.Element {
	t := c.Theme()
	base, hover, pressed, ink, border := t.Surface, t.SurfaceHover, t.SurfacePressed, t.Text, t.Border
	if primary {
		base, hover, pressed, ink, border = t.Accent, t.AccentHover, t.AccentPressed, t.AccentText, ui.Transparent
	}
	el.Height(ControlH()).Padding(0, 12).Gap(8).Radius(RadiusS).
		FontSize(t.FontSize).AlignItems(ui.Center).TextColor(ink)
	if border.A > 0 {
		el.Border(1, border)
	} else {
		el.Border(0, ui.Transparent)
	}
	if !el.IsDisabled() {
		switch {
		case el.Pressed() || active:
			base = pressed
		case el.Hovered():
			base = hover
		}
	}
	return ControlMotion(el).Background(el.AnimateColor("bg", base, MotionFast))
}
