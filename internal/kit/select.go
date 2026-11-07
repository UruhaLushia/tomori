package kit

import (
	"github.com/egoist/mygo/ui"
	"strings"
	"time"
)

type SelectOption[T comparable] struct {
	Value    T
	Label    string
	Detail   string
	Disabled bool
	Hint     string
}

type popupPanelRefs struct {
	panel, anchor ui.Handle
}

// SelectBase uses AttachTo to flip and clamp overlays to the window.
func PopupPanel(c *ui.Context, panel ui.Element, width float32, anchors ...ui.Element) ui.Element {
	_, h := c.Size()
	panel = PopupMotion(c, panel).Width(max(W(160), min(W(320), width))).MaxHeight(min(360, h-16)).Margin(4, 0, 0, 0).Padding(4).Radius(RadiusL).
		Background(Surface).Border(1, LineColor).Shadow(0, 8, 24, 0, Shadow)
	refs := ui.Local(panel, "popup-panel-refs", func() popupPanelRefs { return popupPanelRefs{} })
	panel.Bind(&refs.panel)
	hasAnchor := len(anchors) > 0
	if hasAnchor {
		anchors[0].Bind(&refs.anchor)
	}
	paintPointer := func(p *ui.Painter, r ui.Rect, down bool) {
		bounds := refs.panel.Bounds(c)
		x := r.W / 2
		if hasAnchor {
			anchor := refs.anchor.Bounds(c)
			x = anchor.X + anchor.W/2 - bounds.X
			x = max(6, min(r.W-6, x))
		}
		var path ui.Path
		if down {
			path.MoveTo(r.X+x-4, r.Y).LineTo(r.X+x, r.Y+r.H).LineTo(r.X+x+4, r.Y)
		} else {
			path.MoveTo(r.X+x-4, r.Y+r.H).LineTo(r.X+x, r.Y).LineTo(r.X+x+4, r.Y+r.H)
		}
		path.Close()
		p.FillPath(&path, Surface)
		left, right := ui.Path{}, ui.Path{}
		if down {
			left.MoveTo(r.X+x-4, r.Y).LineTo(r.X+x, r.Y+r.H)
			right.MoveTo(r.X+x, r.Y+r.H).LineTo(r.X+x+4, r.Y)
		} else {
			left.MoveTo(r.X+x-4, r.Y+r.H).LineTo(r.X+x, r.Y)
			right.MoveTo(r.X+x, r.Y).LineTo(r.X+x+4, r.Y+r.H)
		}
		p.StrokePath(&left, 1, LineColor)
		p.StrokePath(&right, 1, LineColor)
	}
	panel.Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(-5).Height(5).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			if !hasAnchor || refs.panel.Bounds(c).Y >= refs.anchor.Bounds(c).Y {
				paintPointer(p, r, false)
			}
		})
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(-5).Height(5).PassThrough().Draw(func(p *ui.Painter, r ui.Rect) {
			if hasAnchor && refs.panel.Bounds(c).Y < refs.anchor.Bounds(c).Y {
				paintPointer(p, r, true)
			}
		})
	})
	return panel
}

func MenuItem(c *ui.Context, el ui.Element, label string, tones ...Tone) ui.Element {
	el.Role(ui.RoleMenuItem).Label(label).FillWidth().Height(ControlH()).Padding(0, 8).Radius(RadiusM).Justify(ui.Start).AlignItems(ui.Center)
	bg := ui.Transparent
	if el.Pressed() {
		bg = Press
	} else if el.Highlighted() || el.Hovered() || el.FocusVisible() {
		bg = Hover
	}
	color := Text
	if len(tones) > 0 && tones[0] == DangerTone {
		color = Red
	}
	return el.Background(bg).Children(func() { Line(c, label, Body).TextColor(color) })
}

func Select[T comparable](c *ui.Context, value *T, label string, options []SelectOption[T], width, popupWidth float32, change func()) ui.Element {
	return selectControl(c, value, label, options, W(max(160, min(200, width))), popupWidth, false, change)
}

func FilterSelect[T comparable](c *ui.Context, value *T, label string, options []SelectOption[T], popupWidth float32, change func()) ui.Element {
	return selectControl(c, value, label, options, 0, popupWidth, strings.Contains(label, "跟随") || strings.Contains(label, "Follow"), change)
}

func FilterPopover(c *ui.Context, label string, derived bool, open *bool, height float32, build func()) ui.Element {
	trigger := ui.ButtonBase(c).Height(ControlH()).Shrink(0).Padding(0, 8, 0, 10).Gap(8).Radius(RadiusS).AlignItems(ui.Center).
		Role(ui.RoleMenuButton).Expanded(*open).OnClick(func() { *open = !*open })
	state := ui.Local(trigger, "filter-layout", func() selectLayout { return selectLayout{} })
	trigger.Bind(&state.handle)
	w, h := c.Size()
	bounds := state.handle.Bounds(c)
	// Updating the filter text may change its width while the panel stays open.
	if *open && state.open && (w != state.w || h != state.h || bounds.X != state.bounds.X || bounds.Y != state.bounds.Y) {
		*open = false
	}
	if state.open && !*open {
		trigger.Focus()
	}
	state.w, state.h, state.bounds, state.open = w, h, bounds, *open
	bg, ink := Surface, Text
	if derived {
		bg, ink = AccentSoft, AccentText
	} else {
		trigger.Border(1, LineColor)
	}
	if trigger.Pressed() || *open {
		bg = Press
	} else if trigger.Hovered() {
		bg = Hover
	}
	shown := middleFit(c, label, W(200))
	if shown != label {
		trigger.Tooltip(label)
	}
	trigger.Background(trigger.AnimateColor("bg", bg, MotionFast)).Children(func() {
		Line(c, shown, Body).TextColor(ink).SingleLine().Shrink(0)
		rotation := float32(0)
		if *open {
			rotation = 180
		}
		ui.Icon(c, ChevronDown).FontSize(IconPx(12)).TextColor(ink).AnimatePose(ui.Pose{Rotate: rotation}, MotionFast)
	})
	ui.PopoverBase(c, trigger, open, func(panel ui.Element) {
		placePopup(c, panel, trigger, W(280), height)
		panel.Children(build)
	})
	return trigger
}

type selectLayout struct {
	handle   ui.Handle
	bounds   ui.Rect
	w, h     float32
	open     bool
	revision int
	prefix   string
	typed    time.Time
	pending  bool
}
type selectKey struct {
	layout   *selectLayout
	revision int
}

func SelectBase[T comparable](c *ui.Context, value *T) ui.SelectParts[T] {
	parts, _ := guardedSelect(c, value)
	return parts
}

func guardedSelect[T comparable](c *ui.Context, value *T) (ui.SelectParts[T], *selectLayout) {
	// Keep native selection and focus behavior; remount only an open popup whose
	// anchor moved, so resize and parent scrolling dismiss it.
	state := ui.Local(ui.Box(c).Absolute().Size(0, 0).PassThrough(), "select-layout", func() selectLayout { return selectLayout{} })
	w, h := c.Size()
	bounds := state.handle.Bounds(c)
	if state.open && (w != state.w || h != state.h || bounds != state.bounds) {
		state.revision++
		state.prefix = ""
	}
	state.w, state.h, state.bounds = w, h, bounds
	c.Key(selectKey{state, state.revision})
	parts := ui.SelectBase(c, value)
	parts.Trigger.Bind(&state.handle)
	state.open = parts.Open()
	if !state.open {
		state.prefix, state.pending = "", false
	}
	return parts, state
}

func selectControl[T comparable](c *ui.Context, value *T, label string, options []SelectOption[T], width, popupWidth float32, derived bool, change func()) ui.Element {
	parts, state := guardedSelect(c, value)
	trigger := parts.Trigger.Height(ControlH()).Shrink(0).Padding(0, 8, 0, 10).Gap(8).Radius(RadiusS).AlignItems(ui.Center).Label(label).OnChange(change)
	if width > 0 {
		trigger.Width(width)
	}
	trigger.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputText || !state.open || ev.Text == "" {
			return false
		}
		if ev.Time.Sub(state.typed) > time.Second {
			state.prefix = ""
		}
		state.prefix += strings.ToLower(ev.Text)
		state.typed = ev.Time
		state.pending = true
		return true
	})
	if state.pending {
		state.pending = false
		for _, option := range options {
			if !option.Disabled && strings.HasPrefix(strings.ToLower(option.Label), state.prefix) {
				parts.Highlight(option.Value)
				break
			}
		}
	}
	bg, ink := Surface, Text
	if derived {
		bg, ink = AccentSoft, AccentText
	} else {
		trigger.Border(1, LineColor)
	}
	if trigger.Pressed() {
		bg = Press
	} else if trigger.Hovered() || trigger.FocusVisible() {
		bg = Hover
	}
	prefix, text, found := strings.Cut(label, " · ")
	if !found {
		text, prefix = label, ""
	}
	prefixWidth := float32(0)
	if prefix != "" {
		prefixWidth = measuredText(c, prefix+" · ") + 1
	}
	maxValue := W(200)
	own := state.handle.Bounds(c).W
	available := own
	if own == 0 {
		available, _ = c.Size()
		available -= 2 * Pad(c)
	}
	if width > 0 {
		available = width
	}
	if prefix != "" && min(maxValue, measuredText(c, text))+prefixWidth+36 > available {
		prefix, prefixWidth = "", 0
		trigger.Tooltip(label)
	}
	maxValue = max(16, min(maxValue, available-prefixWidth-36))
	shown := middleFit(c, text, maxValue)
	trigger.Background(trigger.AnimateColor("bg", bg, MotionFast)).Children(func() {
		spans := []ui.Span{}
		if prefix != "" {
			color := Muted
			if derived {
				color = AccentText.Alpha(.7)
			}
			spans = append(spans, ui.Span{Text: prefix + " · ", Color: color})
		}
		spans = append(spans, ui.Span{Text: shown, Color: ink})
		line := ui.RichText(c, spans...).FontSize(FontPx(Body)).FontWeight(weight(Body)).FixedLineHeight(LinePx(Body)).SingleLine().MinWidth(0)
		if width > 0 {
			line.Grow(1)
		} else {
			line.Shrink(0)
		}
		if shown != text {
			line.Tooltip(text)
		}
		rotation := float32(0)
		if parts.Open() {
			rotation = 180
		}
		ui.Icon(c, ChevronDown).FontSize(IconPx(12)).Shrink(0).TextColor(ink).AnimatePose(ui.Pose{Rotate: rotation}, MotionFast)
	})
	parts.Popup(func(panel ui.Element) {
		longest := W(160)
		for _, option := range options {
			longest = max(longest, measuredText(c, option.Label)+measuredText(c, option.Detail)+48)
		}
		popupHeight := float32(len(options)*int(ControlH()) + 8)
		for _, option := range options {
			if option.Detail != "" {
				popupHeight += 9
				break
			}
		}
		placePopup(c, panel, trigger, max(longest, trigger.Bounds().W), popupHeight)
		ui.Scroll(c).Scrollbars(false).Fill().MinHeight(0).Background(Surface).Children(func() {
			ui.Column(c).FillWidth().Background(Surface).Children(func() {
				for i, option := range options {
					if i == 1 && options[0].Detail != "" {
						Divider(c).Margin(4, 8)
					}
					c.Key(option.Value)
					item := parts.Item(option.Value).Disabled(option.Disabled).Height(ControlH()).FillWidth().Padding(0, 8).Gap(8).Radius(RadiusM).AlignItems(ui.Center).Label(option.Label)
					if option.Hint != "" {
						item.Tooltip(option.Hint)
					}
					selected := *value == option.Value
					style, color, bg := Body, Text, Surface
					if selected {
						style, color, bg = Strong, AccentText, AccentSoft
					} else if item.Highlighted() || item.Hovered() {
						bg = Hover
					}
					item.Background(bg).Children(func() {
						line := Line(c, option.Label, style).TextColor(color).Grow(1)
						if measuredText(c, option.Label)+48 > W(320) {
							line.Tooltip(option.Label)
						}
						if option.Detail != "" {
							Line(c, option.Detail, Caption).MaxWidth(W(120))
						}
						if selected {
							ui.Icon(c, Check).FontSize(IconPx(16)).TextColor(AccentText)
						} else {
							ui.Box(c).Size(16, 16)
						}
					})
				}
			})
		})
	})
	return trigger
}

func measuredText(c *ui.Context, text string) float32 {
	w, _ := c.MeasureText(1<<20, ui.Span{Text: text, Size: FontPx(Body), Weight: weight(Body)})
	return w
}

func middleFit(c *ui.Context, text string, width float32) string {
	if measuredText(c, text) <= width {
		return text
	}
	runes := []rune(text)
	lo, hi := 1, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if measuredText(c, MiddleEllipsis(text, mid)) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return MiddleEllipsis(text, lo)
}

func placePopup(c *ui.Context, panel, anchor ui.Element, width, contentHeight float32) {
	w, h := c.Size()
	width = min(max(W(160), width), min(W(320), w-16))
	r := anchor.Bounds()
	below, above := h-r.Y-r.H-12, r.Y-12
	at, self, available := ui.AnchorBottomStart, ui.AnchorTopStart, below
	if below < min(contentHeight, 240) && above > below {
		at, self, available = ui.AnchorTopStart, ui.AnchorBottomStart, above
	}
	// AttachTo flips horizontal alignment before clamping; offset its resulting
	// position to the shared eight-pixel window inset.
	x := r.X
	overflow := func(x float32) float32 { return max(0, 4-x) + max(0, x+width-(w-4)) }
	if alt := r.X + r.W - width; overflow(alt) < overflow(x) {
		x = alt
	}
	x = max(4, min(x, w-4-width))
	desired := max(8, min(r.X, w-8-width))
	PopupPanel(c, panel, width, anchor).MinWidth(0).MaxWidth(width).AttachTo(anchor, at, self).Left(desired - x).
		Height(min(contentHeight, max(0, min(360, available)))).MaxHeight(max(0, min(360, available)))
	if at == ui.AnchorTopStart {
		panel.Margin(0, 0, 4, 0)
	}
}
