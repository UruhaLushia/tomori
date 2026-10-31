package kit

import (
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"
)

type stepSliderState struct {
	handle          ui.Handle
	value, start    int
	dragging, focus bool
	commitAt        time.Time
}

// StepSlider previews immediately and commits only on release or keyboard idle.
func StepSlider(c *ui.Context, value, lo, hi, step, def int, preview, commit func(int)) ui.Element {
	el := ui.Box(c).Height(ControlH()).MinWidth(W(80)).Focusable().Role(ui.RoleSlider).Value(strconv.Itoa(value)).Range(float64(lo), float64(hi), float64(value)).FocusRing(false)
	s := ui.Local(el, "steps", func() stepSliderState { return stepSliderState{value: value} })
	el.Bind(&s.handle)
	s.value = value
	if s.focus {
		el.Focus()
		s.focus = false
	}
	if !s.commitAt.IsZero() {
		if !s.handle.Focused(c) {
			s.commitAt = time.Time{}
			s.value = s.start
			preview(s.start)
		} else if !c.Now().Before(s.commitAt) {
			s.commitAt = time.Time{}
			commit(s.value)
		} else {
			c.After(s.commitAt.Sub(c.Now()))
		}
	}
	snap := func(v float64) int { return max(lo, min(hi, lo+int(math.Round((v-float64(lo))/float64(step)))*step)) }
	rtl := c.Direction() == ui.RightToLeft
	update := func(x float32) {
		width := max(1, s.handle.Bounds(c).W-16)
		f := max(0, min(1, (x-8)/width))
		if rtl {
			f = 1 - f
		}
		n := snap(float64(lo) + float64(f)*float64(hi-lo))
		if n != s.value {
			s.value = n
			preview(n)
		}
	}
	el.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				return false
			}
			s.start, s.dragging, s.focus = s.value, true, true
			s.commitAt = time.Time{}
			update(ev.X)
			return true
		case ui.InputPointerMove:
			if s.dragging {
				update(ev.X)
				return true
			}
		case ui.InputPointerUp:
			if !s.dragging {
				return false
			}
			s.dragging = false
			if s.handle.Focused(c) {
				update(ev.X)
				commit(s.value)
			} else {
				s.value = s.start
				preview(s.start)
			}
			return true
		case ui.InputKeyDown:
			n := s.value
			switch ev.Key {
			case ui.KeyEscape:
				if !s.dragging && s.commitAt.IsZero() {
					return false
				}
				s.dragging = false
				s.commitAt = time.Time{}
				s.value = s.start
				preview(s.start)
				return true
			case ui.KeyLeft, ui.KeyDown:
				n -= step
			case ui.KeyRight, ui.KeyUp:
				n += step
			case ui.KeyHome:
				n = lo
			case ui.KeyEnd:
				n = hi
			default:
				return false
			}
			if s.commitAt.IsZero() {
				s.start = s.value
			}
			s.value = snap(float64(n))
			preview(s.value)
			s.commitAt = ev.Time.Add(300 * time.Millisecond)
			return true
		}
		return false
	})
	fraction, defaultAt := float32(value-lo)/float32(hi-lo), float32(def-lo)/float32(hi-lo)
	if rtl {
		fraction, defaultAt = 1-fraction, 1-defaultAt
	}
	focused := el.FocusVisible()
	el.Draw(func(p *ui.Painter, r ui.Rect) {
		track := ui.Rect{X: r.X + 8, Y: r.Y + r.H/2 - 2, W: max(0, r.W-16), H: 4}
		p.Fill(track, LineColor, RadiusFull)
		fill := track
		fill.W *= fraction
		if rtl {
			fill.X = track.X + track.W - fill.W
		}
		p.Fill(fill, Accent, RadiusFull)
		p.Fill(ui.Rect{X: track.X + track.W*defaultAt - 1, Y: track.Y + track.H + 4, W: 2, H: 6}, Muted, 1)
		knob := ui.Rect{X: track.X + track.W*fraction - 8, Y: r.Y + r.H/2 - 8, W: 16, H: 16}
		p.Fill(knob, ImageInk, RadiusFull)
		p.Stroke(knob, LineColor, RadiusFull, 1)
		if focused {
			p.Stroke(ui.Rect{X: knob.X - 4, Y: knob.Y - 4, W: 24, H: 24}, Accent, RadiusFull, 2)
		}
	})
	return el
}
