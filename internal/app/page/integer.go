package page

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
)

// Keep partial edits until they form a valid integer or the user submits them.
type integerSetting struct {
	text  string
	model int
	ready bool
}

func (n *integerSetting) input(c *ui.Context, value *int, lo, hi int, width float32, label string, save func(), unit ...string) ui.Element {
	if !n.ready || n.model != *value {
		n.model, n.text, n.ready = *value, strconv.Itoa(*value), true
	}
	input := kit.Input(c, &n.text, unit...).Width(width).Height(kit.ControlH()).Radius(kit.RadiusS).Background(kit.InsetColor).Border(0, ui.Transparent).Label(label).OnChange(func() {
		v, err := strconv.Atoi(strings.TrimSpace(n.text))
		if err == nil && v >= lo && v <= hi && v != *value {
			*value, n.model = v, v
			save()
		}
	}).OnSubmit(func() {
		n.text = strconv.Itoa(*value)
	})
	if v, err := strconv.Atoi(strings.TrimSpace(n.text)); err != nil || v < lo || v > hi {
		input.Border(1, kit.Red)
	}
	return input
}
