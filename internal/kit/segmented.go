package kit

import "github.com/egoist/mygo/ui"

func SegmentedValues[T comparable](c *ui.Context, value *T, options []SelectOption[T], change func()) ui.Element {
	type selection struct {
		model T
		index int
		ready bool
	}
	host := ui.Box(c).Shrink(1).MinWidth(0).MaxWidthPercent(100)
	state := ui.Local(host, "segmented-value", func() selection { return selection{} })
	if !state.ready || state.model != *value {
		state.model, state.index, state.ready = *value, 0, true
		for i, option := range options {
			if option.Value == *value {
				state.index = i
				break
			}
		}
	}
	labels := make([]string, len(options))
	for i, option := range options {
		labels[i] = option.Label
	}
	return host.Children(func() {
		Segmented(c, &state.index, labels...).OnChange(func() {
			*value = options[state.index].Value
			state.model = *value
			if change != nil {
				change()
			}
		})
	})
}

func Segmented(c *ui.Context, selected *int, labels ...string) ui.Element {
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Height(ControlH()).Shrink(0).Padding(4).Gap(4).Radius(RadiusM).Background(InsetColor).Border(0, ui.Transparent)
	track.Children(func() {
		for i, label := range labels {
			segment := parts.Segment(i).Height(H(24, Body)).Shrink(0).Padding(0, 10).Radius(RadiusS).AlignItems(ui.Center).Justify(ui.Center)
			background := ui.Transparent
			if i == *selected {
				background = AccentSoft
			} else if segment.Hovered() || segment.FocusVisible() {
				background = Hover
			}
			ControlMotion(segment).Background(segment.AnimateColor("bg", background, MotionFast)).Border(0, ui.Transparent)
			segment.Children(func() {
				color := Text
				if i == *selected {
					color = AccentText
				}
				style := Body
				if i == *selected {
					style = Strong
				}
				Label(c, label, style).TextColor(segment.AnimateColor("fg", color, MotionFast)).SingleLine()
			})
		}
	})
	return track
}
