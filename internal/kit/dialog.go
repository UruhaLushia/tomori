package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/locales"
)

func DialogHeader(c *ui.Context, title string, close func(), leading ...func()) ui.Element {
	var button ui.Element
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		for _, build := range leading {
			if build != nil {
				build()
			}
		}
		Line(c, title, Heading).Grow(1).ContentTransition(ValueMotion)
		button = IconButton(c, Close, false, close).Size(28, 28).Label(locales.Close()).Tooltip(locales.Close())
	})
	return button
}

func DialogFooter(c *ui.Context, build func()) ui.Element {
	return ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Justify(ui.End).Shrink(0).Children(build)
}
