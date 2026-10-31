package kit

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"math"
	"time"
	"github.com/UruhaLushia/tomori/locales"
)

func TaskIndicator(c *ui.Context, fraction float64, active, failure bool, onClick func()) ui.Element {
	icon, color := Check, Green
	if active || failure {
		icon, color = Download, AccentText
	}
	el := PresenceMotion(ui.ButtonBase(c).Size(28, 28).Shrink(0).Radius(RadiusS).Center().OnClick(onClick))
	if el.Hovered() {
		el.Background(Hover)
	}
	el.Children(func() { ui.Icon(c, icon).FontSize(IconPx(16)).TextColor(color) })
	if active {
		fraction = max(0, min(1, fraction))
		el.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Stroke(ui.Rect{X: r.X + 2, Y: r.Y + 2, W: r.W - 4, H: r.H - 4}, LineSoft, RadiusFull, 2)
			var path ui.Path
			for i := 0; i <= 48; i++ {
				a := -math.Pi/2 + 2*math.Pi*fraction*float64(i)/48
				x, y := r.X+r.W/2+float32(math.Cos(a))*(r.W/2-2), r.Y+r.H/2+float32(math.Sin(a))*(r.H/2-2)
				if i == 0 {
					path.MoveTo(x, y)
				} else {
					path.LineTo(x, y)
				}
			}
			p.StrokePath(&path, 2, Accent)
		})
	}
	if failure {
		el.Children(func() { ui.Box(c).Absolute().Top(1).Right(1).Size(6, 6).Radius(RadiusFull).Background(Red) })
	}
	return el
}

type Identity struct {
	Icon         *ui.Bitmap
	Name, Meta   string
	Switch, Open bool
}

func IdentityHeader(c *ui.Context, id Identity, onToggle func(), actions func()) ui.Element {
	return ui.Row(c).FillWidth().Height(H(44, Heading, Caption)).Shrink(0).Gap(12).AlignItems(ui.Center).Children(func() {
		var trigger ui.Element
		if id.Switch {
			trigger = ui.ButtonBase(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center).Radius(RadiusM).OnClick(onToggle).Label(locales.SwitchInstance()).Expanded(id.Open)
			if trigger.FocusVisible() {
				FocusRing(trigger)
			}
		} else {
			trigger = ui.Row(c).Grow(1).Basis(0).MinWidth(0).Gap(12).AlignItems(ui.Center)
		}
		trigger.Children(func() {
			InstanceIcon(c, id.Icon, 44)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).FillWidth().MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
					Line(c, id.Name, Heading).Shrink(1).Tooltip(id.Name).ContentTransition(ValueMotion)
					if id.Switch {
						bg, ink, rotation := ui.Transparent, Muted, float32(0)
						switch {
						case id.Open:
							bg, ink, rotation = AccentSoft, AccentText, 180
						case trigger.Pressed():
							bg, ink = Press, Text
						case trigger.Hovered() || trigger.FocusVisible():
							bg, ink = Hover, Text
						}
						ui.Box(c).Size(24, 24).Shrink(0).Radius(RadiusS).Center().Background(trigger.AnimateColor("chev-bg", bg, MotionFast)).Children(func() {
							ui.Icon(c, ChevronDown).FontSize(IconPx(16)).TextColor(trigger.AnimateColor("chev-fg", ink, MotionFast)).AnimatePose(ui.Pose{Rotate: rotation}, MotionFast)
						})
					}
				})
				Line(c, id.Meta, Caption).Tooltip(id.Meta).ContentTransition(ValueMotion)
			})
		})
		if actions != nil {
			ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(actions)
		}
	})
}

type Row struct {
	Key                   any
	Lead                  func()
	Title, Subtitle, Tags string
	Badge                 func()
	Selected, Muted       bool
	OnClick               func()
}

func ListRow(c *ui.Context, data Row, trailing func(bool)) ui.Element {
	if data.Key != nil {
		c.Key(data.Key)
	}
	height := RowDoubleH()
	if data.Subtitle == "" {
		height = RowSingleH()
	}
	el := RowButton(c, data.Title, data.Selected, data.OnClick).Height(height).Padding(0, 8).Gap(12).Radius(RadiusM)
	return el.Children(func() {
		if data.Lead != nil {
			data.Lead()
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				color := Text
				if data.Muted {
					color = Muted
				}
				if data.Selected {
					color = AccentText
				}
				title := Line(c, data.Title, Strong).Shrink(1).TextColor(color)
				if data.Subtitle == "" && data.Tags != "" {
					title.Shrink(0)
				}
				if data.Badge != nil {
					data.Badge()
				}
				if data.Tags != "" {
					Line(c, data.Tags, Caption).MinWidth(0).Shrink(100)
				}
			})
			if data.Subtitle != "" {
				Line(c, data.Subtitle, Caption)
			}
		})
		if trailing != nil {
			trailing(el.Hovered() || el.FocusVisible())
		}
	})
}

type TileData struct {
	Icon                               *ui.Bitmap
	Title, Subtitle                    string
	Running, Current, Target, Disabled bool
}

func Tile(c *ui.Context, data TileData, onClick, onLaunch func()) ui.Element {
	el := RowButton(c, data.Title, data.Current, onClick).Height(H(56, Strong, Caption)).Padding(0, 8).Gap(12).Radius(RadiusM)
	if data.Target && !data.Current && !el.Hovered() && !el.Pressed() {
		el.Background(Hover)
	}
	if el.FocusVisible() {
		FocusRing(el)
	}
	return el.Children(func() {
		ui.Box(c).Size(40, 40).Shrink(0).Children(func() {
			InstanceIcon(c, data.Icon, 40)
			if data.Running {
				ui.Box(c).Absolute().Right(-1).Bottom(-1).Size(10, 10).Radius(RadiusFull).Background(Surface).Center().Children(func() { ui.Box(c).Size(6, 6).Radius(RadiusFull).Background(Green) })
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			color := Text
			if data.Current {
				color = AccentText
			}
			Line(c, data.Title, Strong).TextColor(color)
			Line(c, data.Subtitle, Caption)
		})
		ui.Box(c).Width(28).Shrink(0).Children(func() {
			if !data.Current && (el.Hovered() || el.FocusVisible()) && onLaunch != nil {
				QuickAction(c, false, locales.LaunchGame(), onLaunch).Disabled(data.Disabled)
			}
		})
	})
}

func FocusRing(el ui.Element, radii ...float32) ui.Element {
	radius := RadiusM
	if len(radii) > 0 {
		radius = radii[0]
	}
	color, radius := Accent, radius+4
	return el.FocusRing(false).DrawOver(func(p *ui.Painter, r ui.Rect) {
		p.Stroke(ui.Rect{X: r.X - 4, Y: r.Y - 4, W: r.W + 8, H: r.H + 8}, color, radius, 2)
	})
}

type CellData struct {
	Icon                   *ui.Bitmap
	Title, Subtitle, Stats string
	Running                bool
}

func InstanceCell(c *ui.Context, data CellData, onClick func()) ui.Element {
	el := ui.ButtonBase(c).Column().Height(CellH()).FillWidth().Padding(16, 8, 12, 8).Gap(8).AlignItems(ui.Center).Radius(RadiusM).OnClick(onClick).Label(data.Title)
	bg := ui.Transparent
	if el.Pressed() {
		bg = Press
	} else if el.Hovered() || el.FocusVisible() {
		bg = Hover
	}
	if el.FocusVisible() {
		FocusRing(el)
	}
	showCellHint(c, el, data.Stats)
	return LayoutMotion(el).Background(el.AnimateColor("cell-bg", bg, MotionFast)).Children(func() {
		ui.Box(c).Size(48, 48).Shrink(0).Children(func() {
			InstanceIcon(c, data.Icon, 48)
			if data.Running {
				ui.Box(c).Absolute().Right(-1).Bottom(-1).Size(10, 10).Radius(RadiusFull).Background(Surface).Center().Children(func() { ui.Box(c).Size(6, 6).Radius(RadiusFull).Background(Green) })
			}
		})
		ui.Column(c).FillWidth().MinWidth(0).Children(func() {
			Line(c, data.Title, Strong).TextAlign(ui.Center)
			Line(c, data.Subtitle, Caption).TextAlign(ui.Center)
		})
	})
}

func showCellHint(c *ui.Context, el ui.Element, text string) {
	since := ui.Local(el, "cell-hover", func() time.Time { return time.Time{} })
	if !el.Hovered() || text == "" {
		*since = time.Time{}
		return
	}
	if since.IsZero() {
		*since = c.Now()
	}
	if remaining := 500*time.Millisecond - c.Now().Sub(*since); remaining > 0 {
		c.After(remaining)
		return
	}
	ui.Overlay(c, func() {
		ui.Box(c).AttachTo(el, ui.AnchorBottom, ui.AnchorTop).Top(4).Padding(8, 12).Radius(RadiusM).Background(Surface).Border(1, LineColor).Shadow(0, 8, 24, 0, Shadow).PassThrough().Role(ui.RoleTooltip).Children(func() { Line(c, text, Caption).MaxWidth(W(320)) })
	})
}

func InsertionPlaceholder(c *ui.Context, target ui.Element, width, height float32) ui.Element {
	return LayoutMotion(ui.Box(c).AttachTo(target, ui.AnchorTopLeft, ui.AnchorTopLeft).Size(width, height).Radius(RadiusM).Background(AccentSoft).PassThrough().Label(locales.InstanceInsertionPoint()))
}

func MeterBar(c *ui.Context, game, other float64) ui.Element {
	game = max(0, min(1, game))
	other = max(0, min(1-game, other))
	return ui.Row(c).FillWidth().Height(6).Radius(RadiusFull).Background(LineSoft).Clip().Children(func() {
		if game > 0 {
			ui.Box(c).WidthPercent(float32(game) * 100).Height(6).Radius(RadiusFull).Background(Accent).Transition(ui.ElementTransition{Size: !scaleReflow, Animation: &MotionFast})
		}
		if game > 0 && other > 0 {
			ui.Box(c).Width(2).Height(6).Shrink(0)
		}
		if other > 0 {
			ui.Box(c).WidthPercent(float32(other) * 100).Height(6).Radius(RadiusFull).Background(ProgressMuted).Shrink(1).Transition(ui.ElementTransition{Size: !scaleReflow, Animation: &MotionFast})
		}
	})
}

func Meter(c *ui.Context, label string, game, other float64, value, tooltip string, valueWidth float32) ui.Element {
	return ui.Row(c).FillWidth().Height(H(24, Caption)).Gap(8).AlignItems(ui.Center).Tooltip(tooltip).Children(func() {
		Label(c, label, Caption).Width(W(40)).Shrink(0).SingleLine()
		ui.Box(c).Grow(1).MinWidth(16).Children(func() { MeterBar(c, game, other) })
		Label(c, value, Caption).TextColor(Text).Width(valueWidth).Shrink(0).SingleLine().TextAlign(ui.End).ContentTransition(NumberMotion)
	})
}

func MeterLegend(c *ui.Context, game, other string) ui.Element {
	return ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		for i, text := range []string{game, other} {
			color := Accent
			if i == 1 {
				color = ProgressMuted
			}
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() { StatusDot(c, color); Label(c, text, Caption).SingleLine().Shrink(0) })
		}
	})
}

func ChartBar(c *ui.Context, value, peak float64, height float32, today bool) ui.Element {
	return ui.Box(c).FillWidth().Height(height).Children(func() {
		barHeight, color := float32(2), LineColor
		if value > 0 && peak > 0 {
			barHeight, color = max(4, height*float32(value/peak)), ChartMuted
			if today {
				color = Accent
			}
		}
		ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(barHeight).Background(color).Radius(3, 3, 0, 0).Transition(ui.ElementTransition{Size: !scaleReflow, Colors: true, Animation: &MotionFast})
	})
}

func Input(c *ui.Context, value *string, unit ...string) ui.Element {
	el := ui.TextInputBase(c, value).Height(ControlH()).MinWidth(0).Radius(RadiusS).Background(InsetColor).TextColor(Text).FontSize(FontPx(Body)).FontWeight(weight(Body)).Border(0, ui.Transparent).Padding(0, 10)
	if el.FocusVisible() {
		FocusRing(el, RadiusS)
	}
	if len(unit) > 0 && unit[0] != "" {
		width := TextWidth(c, unit[0], Caption)
		el.PaddingEnd(width + 16)
		el.DrawOver(func(p *ui.Painter, r ui.Rect) {
			p.Text(r.X+r.W-width-8, r.Y+(r.H-LinePx(Caption))/2, unit[0], FontPx(Caption), Muted)
		})
	}
	return el
}

func SearchInput(c *ui.Context, query *string, label string, loading ...bool) ui.Element {
	el := Input(c, query).Padding(0, 32).Placeholder(label).Label(label)
	handle := ui.Local(el, "search-input", func() ui.Handle { return ui.Handle{} })
	el.Bind(handle)
	clear := func() { *query = ""; c.Resolve(*handle).SetChanged().Focus() }
	if *query != "" {
		el.OnShortcut(0, ui.KeyEscape, clear)
	}
	el.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Icon(Search, ui.Rect{X: r.X + 8, Y: r.Y + (r.H-16)/2, W: 16, H: 16}, Muted)
	})
	busy := len(loading) > 0 && loading[0]
	if (busy || *query != "") && el.InView() {
		ui.Overlay(c, func() {
			if busy {
				c.Key("search-loading")
				ui.Spinner(c).Size(16, 16).AttachTo(el, ui.AnchorEnd, ui.AnchorEnd).Left(-8).PassThrough()
			} else {
				c.Key("search-clear")
				IconButton(c, Close, false, clear).AttachTo(el, ui.AnchorEnd, ui.AnchorEnd).Left(-4).Size(24, 24).KeepFocus().Label(locales.ClearSearch())
			}
		})
	}
	return el
}

// ListEnd is part of the scroll content, so loading more never shifts the list.
func ListEnd(c *ui.Context, count int, loading bool, failure string, more bool, retry func()) ui.Element {
	return ui.Row(c).FillWidth().Height(H(40, Strong)).Gap(8).Center().Children(func() {
		switch {
		case loading:
			c.Key("more-spinner")
			ui.Spinner(c).Size(16, 16)
			Label(c, locales.LoadingMore(), Caption)
		case failure != "":
			Line(c, failure, Caption).MaxWidth(320)
			Ghost(c, locales.Retry(), retry).Height(TabH())
		case !more:
			Label(c, fmt.Sprintf(locales.ShowingAll(), count), Caption)
		}
	})
}

func NearListEnd(c *ui.Context, state *ui.ListState, count int) bool {
	first, last := state.Visible()
	return state.Bounds(c).H > 0 && count-last <= max(1, last-first)
}

type VersionRowData struct {
	Title, Subtitle, Channel, Badge, Compatibility, Detail string
	Tone                                                   Tone
	Action                                                 string
	Disabled                                               bool
	Installing                                             InstallProgress
}

type InstallProgress struct {
	Active, Canceling bool
	Name, Status      string
	Fraction          float64
	Cancel            func()
}

func InstallTail(c *ui.Context, data InstallProgress, width float32) ui.Element {
	return ui.Row(c).Width(W(width)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
		Progress(c, data.Fraction).Width(W(80)).Height(4).Shrink(1).MinWidth(16)
		Label(c, fmt.Sprintf("%.0f%%", max(0, data.Fraction)*100), Caption).TextColor(Text).Width(W(32)).Shrink(0).TextAlign(ui.End)
		IconButton(c, Close, false, data.Cancel).Size(28, 28).Label(locales.CancelInstallation()).Tooltip(locales.CancelInstallation()).Disabled(data.Canceling)
	})
}

func VersionRow(c *ui.Context, data VersionRowData, action func()) ui.Element {
	if data.Installing.Active {
		data.Subtitle = locales.Installing2() + " " + data.Installing.Name + " · " + data.Installing.Status
	}
	return ListRow(c, Row{Title: data.Title, Subtitle: data.Subtitle, OnClick: action, Lead: func() {
		ui.Box(c).Width(W(48)).Shrink(0).Children(func() {
			if data.Installing.Active {
				c.Key("install-spinner")
				ui.Spinner(c).Size(16, 16)
			} else {
				Badge(c, data.Channel, data.Tone)
			}
		})
	}, Badge: func() {
		if data.Badge != "" {
			Badge(c, data.Badge, AccentTone)
		}
	}}, func(bool) {
		tail := float32(160)
		if !Compact(c) {
			tail = 248
		}
		ui.Row(c).Width(W(tail)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			if data.Installing.Active {
				InstallTail(c, data.Installing, tail)
				return
			}
			if !Compact(c) {
				ui.Column(c).Width(W(160)).Shrink(0).AlignItems(ui.End).Children(func() {
					Line(c, data.Compatibility, Caption)
					Line(c, data.Detail, Caption)
				})
			}
			Button(c, data.Action, action).Height(TabH()).Width(W(80)).Padding(0, 8).Disabled(data.Disabled)
		})
	})
}
