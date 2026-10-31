package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type zoomOutlineState struct {
	window  *mygo.Window
	percent int
}

func (a *app) previewZoom(percent int) {
	if percent == 0 || a.win == nil {
		a.closeZoomOutline()
		return
	}
	percent = min(config.NearestZoom(percent), a.maxZoom())
	if percent == a.zoomApplied {
		a.closeZoomOutline()
		return
	}
	if a.zoomOutline.window != nil && a.zoomOutline.percent == percent {
		return
	}
	a.closeZoomOutline()
	bounds := a.win.ContentBounds()
	target := zoomBounds(bounds, a.zoomApplied, percent)
	color := kit.Accent
	outline := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: a.win, Hidden: true,
		Anchor:    mygo.Rectangle{X: bounds.Width / 2, Y: bounds.Height / 2},
		Placement: mygo.PlacementBottom, Gap: -target.Height/2 - 1,
		Width: target.Width, Height: target.Height,
		Content: ui.View(func(c *ui.Context) {
			c.Root().Background(ui.Transparent)
			ui.Box(c).Fill().Radius(kit.RadiusL).Background(color.Alpha(.05)).Border(2, color.Alpha(.85)).PassThrough()
		}),
	})
	if err := prepareZoomOutline(outline, a.win, target); err != nil {
		outline.Close()
		a.notifyError(locales.UnableToShowTheScalePreview(), err)
		return
	}
	a.zoomOutline = zoomOutlineState{window: outline, percent: percent}
	outline.OnClosed(func() {
		if a.zoomOutline.window == outline {
			a.zoomOutline = zoomOutlineState{}
		}
	})
	outline.ShowInactive()
}

func (a *app) closeZoomOutline() {
	window := a.zoomOutline.window
	a.zoomOutline = zoomOutlineState{}
	if window != nil {
		window.Close()
	}
}

func (a *app) cancelZoomOutline() {
	a.closeZoomOutline()
	a.pages.appearance.CancelZoomPreview()
}
