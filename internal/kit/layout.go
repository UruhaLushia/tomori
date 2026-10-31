package kit

import "github.com/egoist/mygo/ui"

const ScrollbarGutter = 0

const (
	Bleed         = 8
	RowSingle     = 40
	RowDouble     = 48
	FormRowMin    = 52
	ControlSmallH = 28
	TileMinWidth  = 240
	CellMinWidth  = 136
)

const (
	S4  = 4
	S8  = 8
	S12 = 12
	S16 = 16
	S24 = 24
	S32 = 32

	PagePad = S16 // Deprecated: use Pad(c) for page geometry.
)

func Compact(c *ui.Context) bool {
	w, h := c.Size()
	return w < 860 || h < 620
}

// CompactWidth keeps horizontal responsive layouts stable while a window is
// being resized vertically.
func CompactWidth(c *ui.Context) bool {
	w, _ := c.Size()
	return w < 860
}

// Page spacing stays fixed when responsive content changes at a breakpoint.
func Pad(c *ui.Context) float32 { return S16 }

func Gap(c *ui.Context) float32 { return Pad(c) }

func ColGap(c *ui.Context) float32 { return S16 }

func Page(c *ui.Context, build func()) ui.Element {
	pad := Pad(c)
	return ui.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(pad, pad, 0, pad).Gap(Gap(c)).Children(build)
}

func PageScroll(c *ui.Context, build func()) ui.Element {
	pad := Pad(c)
	return ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(Gap(c)).Padding(pad, pad, pad, pad).Children(build)
	})
}

func PageHost(c *ui.Context, title string, scroll bool, build func()) ui.Element {
	if scroll {
		return PageScroll(c, func() {
			if title != "" {
				PageTitle(c, title, "", nil)
			}
			build()
		})
	}
	return Page(c, build)
}

func PageTitle(c *ui.Context, title, meta string, trailing func()) ui.Element {
	return ui.Row(c).FillWidth().Height(H(40, Heading)).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		Label(c, title, Heading)
		if meta != "" {
			Label(c, meta, Caption)
		}
		ui.Box(c).Grow(1).MinWidth(0)
		if trailing != nil {
			trailing()
		}
	})
}

func Section(c *ui.Context, title string, trailing func()) ui.Element {
	return ui.Row(c).FillWidth().Height(H(24, Caption)).AlignItems(ui.Center).Children(func() {
		Label(c, title, SectionText).Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

func Toolbar(c *ui.Context, build func()) ui.Element {
	return ui.Row(c).FillWidth().Wrap().Gap(8).AlignItems(ui.Center).Shrink(0).Children(build)
}

func SearchField(c *ui.Context, query *string, label string, loading ...bool) ui.Element {
	el := SearchInput(c, query, label, loading...).Shrink(0)
	width := ui.Local(el, "search-field-width", func() float32 { return 240 })
	if taken := el.LayoutBounds().W; taken > 0 {
		rowWidth, _ := c.Size()
		free := rowWidth - taken
		if *width == 240 && free < 0 {
			*width = 160
		} else if *width == 160 && free >= 80 {
			*width = 240
		}
	}
	return el.Width(*width)
}

func Group(c *ui.Context, title string, fn func()) ui.Element {
	return FormGroup(c, title, nil, fn)
}

func Card(c *ui.Context, el ui.Element) ui.Element {
	return LayoutMotion(el.FillWidth().Radius(RadiusL).Background(Surface).Border(1, LineSoft))
}

func SettingRow(c *ui.Context, label, hint string, control func()) ui.Element {
	row := LayoutMotion(ui.Row(c.Key(settingMotionKey(label))).FillWidth().MinHeight(H(52, Body, Caption)).Padding(8, 0).Gap(16).AlignItems(ui.Center))
	return row.Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(W(120)).Children(func() {
			Line(c, label, Body)
			if hint != "" {
				Label(c, hint, Caption).MaxLines(2).Wrap()
			}
		})
		ui.Row(c).Shrink(0).MaxWidthPercent(70).Wrap().Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(control)
	})
}

type headerFit struct {
	title, tabs, actions ui.Handle
	titleW, tabsW, actW  float32
	inline               bool
}

func PageHeader(c *ui.Context, title, count string, tabs, actions func()) ui.Element {
	fit := ui.Local(ui.Box(c).Absolute().Size(0, 0).PassThrough(), "page-header-fit", func() headerFit { return headerFit{inline: true} })
	for _, part := range []struct {
		handle *ui.Handle
		width  *float32
	}{{&fit.title, &fit.titleW}, {&fit.tabs, &fit.tabsW}, {&fit.actions, &fit.actW}} {
		if w := part.handle.Bounds(c).W; w > 0 {
			*part.width = w
		}
	}
	width, _ := c.Size()
	if fit.tabsW > 0 {
		fit.inline = fit.tabsW <= width-fit.titleW-fit.actW-16
	}
	return ui.Column(c).FillWidth().Shrink(0).Children(func() {
		ui.Row(c).FillWidth().Height(H(40, Heading)).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Bind(&fit.title).Children(func() {
				if title != "" {
					Label(c, title, Heading).Shrink(0)
				}
				if count != "" {
					Label(c, count, Caption).Shrink(0)
				}
			})
			if tabs != nil && fit.inline {
				ui.Row(c).Shrink(0).Bind(&fit.tabs).Children(tabs)
			}
			ui.Spacer(c)
			if actions != nil {
				ui.Row(c).Gap(8).Shrink(0).AlignItems(ui.Center).Bind(&fit.actions).Children(actions)
			}
		})
		if tabs != nil && !fit.inline {
			ui.Row(c).FillWidth().Margin(12, 0, 0, 0).Bind(&fit.tabs).Children(tabs)
		}
	})
}

func ScrolledEdge(el ui.Element) ui.Element {
	color := LineSoft
	return el.DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, color, 0)
	})
}

func SectionHeader(c *ui.Context, title string, trailing func()) ui.Element {
	return Section(c, title, trailing)
}

func FormGroup(c *ui.Context, title string, trailing, rows func()) ui.Element {
	if title == "" {
		panic("kit.FormGroup: a group requires a title")
	}
	return PresenceMotion(ui.Column(c.Key(groupMotionKey(title))).FillWidth().Shrink(0).Gap(4)).Children(func() {
		SectionHeader(c, title, trailing)
		ui.Column(c).FillWidth().Padding(0, 16).Radius(RadiusM).Background(Surface).Border(1, LineSoft).Children(rows)
	})
}

// FormColumn keeps the header and all setting groups on the same centered grid.
func FormColumn(c *ui.Context, build func()) ui.Element {
	return ui.Column(c).FillWidth().MaxWidth(1120).AlignSelf(ui.Center).MinWidth(0).MinHeight(0).Children(build)
}

func Field(c *ui.Context, label string, control func()) ui.Element {
	return ui.Column(c).FillWidth().Shrink(0).Gap(4).Children(func() {
		Label(c, label, SectionText)
		control()
	})
}

func FormRow(c *ui.Context, label, hint string, control func()) ui.Element {
	return SettingRow(c, label, hint, control)
}

func FormWideRow(c *ui.Context, label, value string, body func()) ui.Element {
	return LayoutMotion(ui.Column(c.Key(settingMotionKey(label))).FillWidth().Padding(12, 0).Gap(8)).Children(func() {
		if label != "" {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				Line(c, label, Body).Grow(1)
				if value != "" {
					Label(c, value, Caption).SingleLine().Shrink(0)
				}
			})
		}
		body()
	})
}

func FormValueRow(c *ui.Context, label, value string, action func()) ui.Element {
	return FormRow(c, label, "", func() {
		Line(c, MiddleEllipsis(value, 54), BodyMuted).MaxWidth(W(420)).Shrink(1).Tooltip(value).Selectable()
		if action != nil {
			action()
		}
	})
}

func BackBar(c *ui.Context, parent string, onBack func(), trailing func()) ui.Element {
	return ui.Row(c).FillWidth().Height(ControlH()).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		back := BackButton(c, parent, onBack).Margin(0, 0, 0, -8)
		focused := ui.Local(back, "detail-focus", func() bool { return false })
		if !*focused {
			back.Focus()
			*focused = true
		}
		ui.Spacer(c)
		if trailing != nil {
			trailing()
		}
	})
}

func MiddleEllipsis(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	left := (limit - 1) / 2
	return string(runes[:left]) + "…" + string(runes[len(runes)-(limit-left-1):])
}

func DetailRow(c *ui.Context, label, value string, control func()) ui.Element {
	return ui.Row(c).FillWidth().Padding(10, 2).Gap(12).AlignItems(ui.Center).Children(func() {
		Line(c, label, Caption).Shrink(0)
		Label(c, value, Caption).Grow(1).MinWidth(0).MaxLines(2).Wrap().Selectable()
		if control != nil {
			control()
		}
	})
}

func Divider(c *ui.Context) ui.Element {
	return ui.Divider(c)
}

func Scroll(c *ui.Context, fn func()) ui.Element {
	pad := Pad(c)
	return ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().Children(func() {
		ui.Column(c).FillWidth().Shrink(0).Gap(S24).Padding(0, ScrollbarGutter, pad, 0).Children(fn)
	})
}

func ErrorText(c *ui.Context, text string) ui.Element {
	return FoldMotion(Label(c.Key(motionStateKey("error")), text, Caption).TextColor(Red).MaxLines(3).Wrap()).ContentTransition(ValueMotion)
}
