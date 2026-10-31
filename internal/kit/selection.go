package kit

import "github.com/egoist/mygo/ui"

func SelectionBox(c *ui.Context, selected, mixed bool, label string, change func()) ui.Element {
	box := ui.CheckboxBase(c, &selected).Size(24, 24).Shrink(0).Center().Radius(RadiusS).Label(label).OnClick(change)
	if mixed {
		box.Mixed()
	}
	box.Children(func() {
		mark := ui.Box(c).Size(16, 16).Radius(RadiusS).Center()
		if selected || mixed {
			mark.Background(Accent).Children(func() {
				icon := Check
				if mixed {
					icon = Minus
				}
				ui.Icon(c, icon).FontSize(IconPx(12)).TextColor(AccentInk)
			})
		} else {
			mark.Background(Surface).Border(1, LineColor.Mix(Muted, .35))
			if box.Hovered() {
				mark.BorderColor(Accent)
			}
		}
	})
	return box
}
