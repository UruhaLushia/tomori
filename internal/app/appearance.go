package app

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func configureGPU(mode int) {
	switch mode {
	case config.GPUOn:
		_ = os.Setenv("MYGO_GPU", "1")
	case config.GPUOff:
		_ = os.Setenv("MYGO_GPU", "0")
	default:
		_ = os.Unsetenv("MYGO_GPU")
	}
}

func (a *app) gpuChanged() {
	configureGPU(a.cfg.Appearance.GPU)
	a.saveConfig()
	if a.win != nil {
		a.win.Update(a.recreateMainWindow)
	}
}

type zoomPreview struct {
	percent  int
	deadline time.Time
}

func (a *app) zoomPercent() int {
	if a.zoomPending != nil {
		return a.zoomPending.percent
	}
	return config.NearestZoom(a.cfg.Appearance.UIZoom)
}

func (a *app) refreshZoomScreen() {
	area := mygo.Screen.DisplayMatching(a.win.Bounds()).WorkArea
	a.win.Update(func() { a.zoomWorkArea = area })
}

func (a *app) maxZoom() int {
	area := a.zoomWorkArea
	if area.Width <= 0 || area.Height <= 0 {
		return 200
	}
	limit := 75
	for _, step := range config.ZoomSteps {
		z := float64(step) / 100
		if float64(area.Width)/z >= minimumWindowWidth && float64(area.Height)/z >= minimumWindowHeight {
			limit = step
		}
	}
	return limit
}

func (a *app) applyZoom(c *ui.Context) {
	if pending := a.zoomPending; pending != nil {
		if left := pending.deadline.Sub(c.Now()); left > 0 {
			c.After(left)
		} else {
			a.zoomPending = nil
			c.CloseToast("ui-zoom")
		}
	}
	percent := min(a.zoomPercent(), a.maxZoom())
	z := float32(percent) / 100
	c.SetZoom(z)
	if a.win == nil || a.zoomApplied == percent {
		return
	}
	previous := a.zoomApplied
	a.zoomApplied = percent
	w, h := int(math.Ceil(float64(minimumWindowWidth*z))), int(math.Ceil(float64(minimumWindowHeight*z)))
	bounds := a.win.ContentBounds()
	target := zoomBounds(bounds, previous, percent)
	a.win.SetMinimumSize(w, h)
	if previous > 0 || bounds.Width < w || bounds.Height < h {
		dx, dy := float64(bounds.Width-target.Width)/2, float64(bounds.Height-target.Height)/2
		a.closeZoomOutline()
		a.win.SetContentBounds(target)
		a.moveNiriWindow(dx, dy)
	}
}

func zoomBounds(bounds mygo.Rectangle, from, to int) mygo.Rectangle {
	if from <= 0 {
		from = to
	}
	w := max(int(math.Round(float64(bounds.Width)*float64(to)/float64(from))), int(math.Ceil(float64(minimumWindowWidth)*float64(to)/100)))
	h := max(int(math.Round(float64(bounds.Height)*float64(to)/float64(from))), int(math.Ceil(float64(minimumWindowHeight)*float64(to)/100)))
	bounds.X += int(math.Round(float64(bounds.Width-w) / 2))
	bounds.Y += int(math.Round(float64(bounds.Height-h) / 2))
	bounds.Width, bounds.Height = w, h
	return bounds
}

func (a *app) changeZoom(percent int) {
	a.closeZoomOutline()
	percent = min(config.NearestZoom(percent), a.maxZoom())
	if percent == a.zoomPercent() {
		return
	}
	pending := &zoomPreview{percent: percent, deadline: time.Now().Add(10 * time.Second)}
	a.zoomPending = pending
	undo := func() {
		if a.zoomPending == pending {
			a.zoomPending = nil
		}
	}
	a.enqueueNotice(ui.Toast{
		ID: "ui-zoom", Title: fmt.Sprintf(locales.InterfaceScaleChangedTo(), percent),
		Type: string(kit.FeedbackSuccess), Timeout: -1,
		Data: kit.ToastConfirmation{Deadline: pending.deadline, OnConfirm: func() {
			if a.zoomPending == pending {
				a.cfg.Appearance.UIZoom = pending.percent
				a.zoomPending = nil
				a.saveConfig()
			}
		}},
		Action: locales.Undo(), OnAction: undo, OnClose: undo,
	})
}

func (a *app) appearanceSource() mygo.ThemeSource {
	switch a.cfg.Appearance.ColorMode {
	case config.ColorLight:
		return mygo.ThemeLight
	case config.ColorDark:
		return mygo.ThemeDark
	default:
		return mygo.ThemeSystem
	}
}

func (a *app) appearanceChanged() {
	source := a.appearanceSource()
	if mygo.Theme.Source() != source {
		mygo.Theme.SetSource(source)
	}
	a.saveConfig()
}

func (a *app) applyAppearance(dark bool) {
	cfg := a.cfg.Appearance
	kit.Apply(dark, kit.ParseColor(cfg.Accent))
	kit.SetFonts(cfg.Font, cfg.LogFont)
	kit.SetFontScale(cfg.FontScale)
	kit.SetFontWeight(cfg.FontWeight)
}

func (a *app) loadFonts() {
	if a.fontsLoading || a.fonts != nil {
		return
	}
	a.fontsLoading = true
	a.workers.Go(func() {
		families := ui.FontFamilies()
		names := make([]string, 0, len(families)+1)
		for _, family := range families {
			names = append(names, family.Name)
		}
		a.win.Update(func() {
			a.fontsLoading = false
			if a.ctx.Err() == nil {
				a.fonts = names
			}
		})
	})
}
