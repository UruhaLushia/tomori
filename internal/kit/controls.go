package kit

import "github.com/egoist/mygo/ui"

func Switch(c *ui.Context, on *bool) ui.Element {
	el := ui.SwitchBase(c, on).Size(32, 18).Shrink(0).Radius(RadiusFull)
	target := float32(0)
	if *on {
		target = 1
	}
	position := el.Animate("thumb", target, motionFast)
	el.Background(Faint.Mix(Accent, position))
	if c.Direction() == ui.RightToLeft {
		position = 1 - position
	}
	el.Draw(func(p *ui.Painter, r ui.Rect) {
		p.Fill(ui.Rect{X: r.X + 2 + position*14, Y: r.Y + 2, W: 14, H: 14}, ImageInk, RadiusFull)
	})
	if el.FocusVisible() {
		FocusRing(el, RadiusFull)
	}
	return el
}

func Slider(c *ui.Context, value *float64, lo, hi float64) ui.Element {
	el := ui.SliderBase(c, value, lo, hi).Height(ControlH()).MinWidth(80).Padding(0, 8).FocusRing(false)
	fraction := float32(0)
	if hi > lo {
		fraction = float32(max(0, min(1, (*value-lo)/(hi-lo))))
	}
	rtl := c.Direction() == ui.RightToLeft
	if rtl {
		fraction = 1 - fraction
	}
	focused := el.FocusVisible()
	el.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + 8, Y: r.Y + r.H/2 - 2, W: max(0, r.W-16), H: 4}
		x := track.X + track.W*fraction
		p.Fill(track, LineColor, RadiusFull)
		fill := track
		fill.W = x - track.X
		if rtl {
			fill.X, fill.W = x, track.X+track.W-x
		}
		p.Fill(fill, Accent, RadiusFull)
		knob := ui.Rect{X: x - 8, Y: r.Y + r.H/2 - 8, W: 16, H: 16}
		p.Fill(knob, ImageInk, RadiusFull)
		p.Stroke(knob, LineColor, RadiusFull, 1)
		if focused {
			p.Stroke(ui.Rect{X: knob.X - 4, Y: knob.Y - 4, W: 24, H: 24}, Accent, RadiusFull, 2)
		}
	})
	return el
}
