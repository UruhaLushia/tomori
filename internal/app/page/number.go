package page

import "github.com/egoist/mygo/ui"
import "github.com/UruhaLushia/tomori/internal/kit"

type numberSetting struct{}

func (n *numberSetting) input(c *ui.Context, value *int, lo, hi, step int, save func()) {
	kit.NumberField(c, value, lo, hi, step, "", "", save)
}
