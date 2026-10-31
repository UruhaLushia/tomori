package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
)

type TextStyle int

const (
	Heading     TextStyle = iota // 20
	Strong                       // 13
	Body                         // 13
	BodyMuted                    // 13, muted
	Caption                      // 11, muted
	SectionText                  // 11, muted
	Mono                         // 12, log font

)

var fontWeight = config.DefaultFontWeight

func SetFontWeight(value int) {
	if value == 0 {
		value = config.DefaultFontWeight
	}
	if value < 100 || value > 900 || value%100 != 0 {
		value = config.DefaultFontWeight
	}
	if fontWeight != value {
		fontWeight = value
		cachedTheme = nil
	}
}

func weight(style TextStyle) int {
	offset := 0
	switch style {
	case Strong, SectionText:
		offset = 200
	case Heading:
		offset = 300
	}
	return min(900, fontWeight+offset)
}

func (style TextStyle) Apply(el ui.Element) ui.Element {
	return el.FontSize(FontPx(style)).FontWeight(weight(style)).FixedLineHeight(LinePx(style))
}

func TextWidth(c *ui.Context, text string, style TextStyle) float32 {
	w, _ := c.MeasureText(1<<20, ui.Span{Text: text, Size: FontPx(style), Weight: weight(style)})
	return w
}

func Label(c *ui.Context, s string, style TextStyle) ui.Element {
	t := style.Apply(ui.Text(c, s))
	switch style {
	case BodyMuted:
		t.TextColor(Muted)
	case Caption:
		t.TextColor(Muted)
	case Heading:
		t.LetterSpacing(-.2)
	case SectionText:
		t.TextColor(Muted).LetterSpacing(.4)
	case Mono:
		t.Font(LogFont())
	}
	return t
}

func Line(c *ui.Context, s string, style TextStyle) ui.Element {
	return Label(c, s, style).MinWidth(0).MaxLines(1).Ellipsis("…")
}

func TextLine(c *ui.Context, s string, style TextStyle) ui.Element {
	return Line(c, s, style)
}

func Preheat(c *ui.Context, s string, size float32, log, strong bool) ui.Element {
	t := ui.Text(c, s).FontSize(textSize(size)).FontWeight(weight(Body)).MaxLines(1)
	if log {
		t.Font(LogFont())
	}
	if strong {
		t.FontWeight(weight(Strong))
	}
	return t
}

func TextArea(c *ui.Context, value *string, style TextStyle) ui.Element {
	el := ui.TextArea(c, value).FontSize(FontPx(style)).FontWeight(weight(style)).FixedLineHeight(LinePx(style))
	if style == Mono {
		el.Font(LogFont())
	}
	return el
}

func styleSize(style TextStyle) float32 {
	switch style {
	case Body, BodyMuted, Strong:
		return 13
	case Caption, SectionText:
		return 11
	case Mono:
		return 12
	case Heading:
		return 20
	default:
		return 13
	}
}
