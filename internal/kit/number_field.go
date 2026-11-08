package kit

import (
	"math"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

type numberFieldState struct {
	text                    string
	model                   int
	ready, focused, pending bool
	handle                  ui.Handle
}

// NumberField commits on Enter, blur, or a focused step with the keyboard or wheel.
func NumberField(c *ui.Context, value *int, lo, hi, step int, unit, label string, commit func()) ui.Element {
	host := ui.Box(c).Shrink(0)
	s := ui.Local(host, "number", func() numberFieldState { return numberFieldState{} })
	if !s.ready || s.model != *value {
		s.text, s.model, s.ready = strconv.Itoa(*value), *value, true
	}
	step = max(1, step)
	set := func(v int) {
		v = max(lo, min(hi, v))
		v = max(lo, min(hi, lo+int(math.Round((float64(v)-float64(lo))/float64(step)))*step))
		s.text, s.model = strconv.Itoa(v), v
		if v != *value {
			*value = v
			s.pending = true
		}
	}
	parse := func() {
		if v, err := strconv.Atoi(strings.TrimSpace(s.text)); err == nil {
			set(v)
		} else {
			s.text = strconv.Itoa(*value)
		}
	}
	if s.focused && !s.handle.Focused(c) {
		parse()
	}
	if s.pending {
		s.pending = false
		if commit != nil {
			commit()
		}
	}
	var input ui.Element
	host.Children(func() {
		input = Input(c, &s.text, unit).Bind(&s.handle).Width(W(88)).Label(label).OnSubmit(parse)
		s.focused = input.Focused()
		input.HandleInput(func(ev ui.InputEvent) bool {
			switch {
			case ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyUp:
				set(*value + step)
			case ev.Kind == ui.InputKeyDown && ev.Key == ui.KeyDown:
				set(*value - step)
			case ev.Kind == ui.InputScroll && ev.DY != 0 && s.handle.Focused(c):
				if ev.DY < 0 {
					set(*value + step)
				} else {
					set(*value - step)
				}
			default:
				return false
			}
			return true
		})
		if v, err := strconv.Atoi(strings.TrimSpace(s.text)); err != nil || v < lo || v > hi {
			input.Border(1, Red)
		}
	})
	return input
}
