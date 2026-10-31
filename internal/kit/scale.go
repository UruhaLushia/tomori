package kit

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
)

var fontScale = float32(1)

func SetFontScale(percent int) {
	if percent == 0 {
		percent = 100
	}
	s := float32(max(75, min(config.MaxFontScale, percent))) / 100
	if fontScale != s {
		fontScale = s
		cachedTheme = nil
	}
}

func WithFontScale(c *ui.Context, percent int, build func()) {
	previous := fontScale
	fontScale = float32(max(75, min(config.MaxFontScale, percent))) / 100
	defer func() { fontScale = previous; cachedTheme = nil }()
	build()
}

var scaleReflow bool

func LayoutAnimation(animation ui.Animation) ui.Animation {
	if scaleReflow {
		return ui.Animation{Duration: time.Nanosecond}
	}
	return animation
}

// Suppress positional transitions while font metrics or the viewport settle.
func BeginFrame(c *ui.Context) func() {
	// Keep the overlay parent alive for the first popup's entry and final exit.
	ui.Overlay(c, func() {})
	type state struct {
		scale float32
		w, h  float32
		until time.Time
	}
	w, h := c.Size()
	s := ui.Local(ui.Box(c).Absolute().Size(0, 0).PassThrough(), "font-layout", func() state { return state{scale: fontScale, w: w, h: h} })
	if s.scale != fontScale || s.w != w || s.h != h {
		s.scale = fontScale
		s.w, s.h = w, h
		s.until = c.Now().Add(motionUnfold)
	}
	scaleReflow = c.Now().Before(s.until)
	if scaleReflow {
		// Keep transitions registered so resizing does not create exit copies.
		c.DisableAnimations()
		c.After(s.until.Sub(c.Now()))
	}
	return func() { scaleReflow = false }
}

func round(v float32) float32        { return float32(math.Round(float64(v))) }
func textSize(base float32) float32  { return max(9, base*fontScale) }
func FontPx(style TextStyle) float32 { return textSize(styleSize(style)) }
func baseLine(style TextStyle) float32 {
	switch style {
	case Caption, SectionText:
		return 16
	case Heading:
		return 24
	default:
		return 20
	}
}
func LinePx(style TextStyle) float32 {
	return round(max(baseLine(style), FontPx(style)*1.2))
}
func H(base float32, styles ...TextStyle) float32 {
	height := base
	for _, style := range styles {
		height += max(0, LinePx(style)-baseLine(style))
	}
	return round(height)
}
func W(base float32) float32 { return base }

// Deprecated: icons no longer follow font size (T-F-6); remove next version.
func IconPx(base float32) float32 { return base }
func ControlH() float32           { return H(32, Body) }
func TabH() float32               { return H(28, Body) }
func RowSingleH() float32         { return H(40, Strong) }
func RowDoubleH() float32         { return H(48, Strong, Caption) }
func CellH() float32              { return H(120, Strong, Caption) }
