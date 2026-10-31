package kit

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

var AccentPresets = []string{"#397ef5", "#9959df", "#dc6a9c", "#d18c42", "#4d9a79"}

func ColorSwatch(c *ui.Context, color ui.Color, selected bool, click func()) ui.Element {
	el := ui.ButtonBase(c).Size(20, 20).Shrink(0).Radius(RadiusFull).Background(color).OnClick(click).Label(HexColor(color))
	if selected || el.FocusVisible() {
		el.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Stroke(ui.Rect{X: r.X - 2, Y: r.Y - 2, W: r.W + 4, H: r.H + 4}, Surface, RadiusFull, 2)
			p.Stroke(ui.Rect{X: r.X - 4, Y: r.Y - 4, W: r.W + 8, H: r.H + 8}, color, RadiusFull, 2)
		})
	}
	return el
}

func HexColor(c ui.Color) string {
	if c.A != 255 {
		return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
	}
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

// APCA-W3 0.1.9.
func apcaY(c ui.Color) float64 {
	linear := func(v uint8) float64 { return math.Pow(float64(v)/255, 2.4) }
	y := 0.2126729*linear(c.R) + 0.7151522*linear(c.G) + 0.0721750*linear(c.B)
	if y < 0.022 {
		y += math.Pow(0.022-y, 1.414)
	}
	return y
}

func apcaLc(text, background ui.Color) float64 {
	yt, yb := apcaY(text), apcaY(background)
	if math.Abs(yb-yt) < 0.0005 {
		return 0
	}
	if yb > yt {
		s := (math.Pow(yb, 0.56) - math.Pow(yt, 0.57)) * 1.14
		if s < 0.1 {
			return 0
		}
		return (s - 0.027) * 100
	}
	s := (math.Pow(yb, 0.65) - math.Pow(yt, 0.62)) * 1.14
	if s > -0.1 {
		return 0
	}
	return (s + 0.027) * 100
}

var inkWhite, inkDark = ui.Hex("#ffffff"), ui.Hex("#211c16")

func accentInk(background ui.Color) ui.Color {
	if math.Abs(apcaLc(inkWhite, background)) >= math.Abs(apcaLc(inkDark, background)) {
		return inkWhite
	}
	return inkDark
}

func accentText(accent, surface, text ui.Color) ui.Color {
	for i := 0; i <= 10; i++ {
		color := accent.Mix(text, float32(i)/10)
		if math.Abs(apcaLc(color, surface)) >= 50 {
			return color
		}
	}
	return text
}
