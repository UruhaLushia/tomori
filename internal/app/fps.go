package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

const (
	fpsSampleInterval = 250 * time.Millisecond
	fpsChartDuration  = 10 * time.Second
	fpsSampleLimit    = int(fpsChartDuration / fpsSampleInterval)
)

type fpsSample struct {
	fps   float64
	frame float64
}

func (a *app) fpsChanged() {
	a.saveConfig()
	if a.cfg.Appearance.ShowFPS {
		a.openFPSWindow()
	} else {
		a.closeFPSWindow()
	}
}

func (a *app) openFPSWindow() {
	if a.win == nil || a.fpsWindow != nil {
		return
	}
	a.resetFPS()
	w := mygo.NewWindow(mygo.WindowOptions{
		Title:           locales.FpsDebug(),
		Width:           230,
		Height:          150,
		MinWidth:        200,
		MinHeight:       135,
		Frameless:       true,
		CornerRadius:    8,
		DisableResize:   true,
		Parent:          a.win,
		AlwaysOnTop:     true,
		SkipTaskbar:     true,
		BackgroundColor: kit.HexColor(kit.Window),
		Content:         ui.View(a.fpsDebugView),
	})
	a.fpsWindow = w
	w.OnClosed(func() {
		if a.fpsWindow == w {
			a.fpsWindow = nil
		}
		closing := a.fpsWindowClosing
		a.fpsWindowClosing = false
		if !closing && a.cfg != nil && a.cfg.Appearance.ShowFPS {
			a.cfg.Appearance.ShowFPS = false
			a.saveConfig()
			if a.win != nil {
				a.win.Invalidate()
			}
		}
	})
}

func (a *app) closeFPSWindow() {
	if a.fpsWindow == nil {
		return
	}
	a.fpsWindowClosing = true
	w := a.fpsWindow
	a.fpsWindow = nil
	w.Close()
}

func (a *app) resetFPS() {
	a.fpsMu.Lock()
	a.fpsFrameStart = time.Time{}
	a.fpsFrameCount = 0
	a.fpsWarmup = false
	a.fpsSamples = nil
	a.fpsAccelerated = false
	a.fpsMu.Unlock()
}

func (a *app) fpsMeter(c *ui.Context) {
	if !a.cfg.Appearance.ShowFPS {
		return
	}
	ui.Box(c).Absolute().Fill().PassThrough().Draw(func(p *ui.Painter, _ ui.Rect) {
		// Keep requesting frames independently of GTK's sometimes stale 60 Hz
		// frame-clock rate; the compositor still presents at the display rate.
		p.AnimationFrame()
		p.Accelerated()
		a.fpsMu.Lock()
		a.fpsAccelerated = p.Accelerated()
		a.fpsMu.Unlock()
		a.recordFPSFrame(p.Now())
	})
}

func (a *app) recordFPSFrame(now time.Time) {
	a.fpsMu.Lock()
	if a.fpsFrameStart.IsZero() {
		a.fpsFrameStart = now
	}
	a.fpsFrameCount++
	if elapsed := now.Sub(a.fpsFrameStart); elapsed >= fpsSampleInterval {
		sample := fpsSample{
			fps:   float64(a.fpsFrameCount) / elapsed.Seconds(),
			frame: elapsed.Seconds() * 1000 / float64(a.fpsFrameCount),
		}
		// The first interval includes opening the debug window and is often a
		// startup transient rather than the steady-state frame rate.
		if a.fpsWarmup {
			a.fpsSamples = append(a.fpsSamples, sample)
		} else {
			a.fpsWarmup = true
		}
		if len(a.fpsSamples) > fpsSampleLimit {
			a.fpsSamples = a.fpsSamples[len(a.fpsSamples)-fpsSampleLimit:]
		}
		a.fpsFrameStart = now
		a.fpsFrameCount = 0
	}
	a.fpsMu.Unlock()
}

func (a *app) fpsSamplesCopy() []fpsSample {
	a.fpsMu.Lock()
	defer a.fpsMu.Unlock()
	return append([]fpsSample(nil), a.fpsSamples...)
}

func (a *app) fpsDebugView(c *ui.Context) {
	c.SetTheme(kit.Theme())
	c.After(100 * time.Millisecond)
	samples := a.fpsSamplesCopy()
	latest := fpsSample{}
	if len(samples) > 0 {
		latest = samples[len(samples)-1]
	}
	a.fpsMu.Lock()
	accelerated := a.fpsAccelerated
	a.fpsMu.Unlock()
	ui.Column(c).Fill().Padding(5).Gap(3).Background(kit.Surface).DragWindow().Children(func() {
		ui.Row(c).FillWidth().Height(15).AlignItems(ui.Center).Gap(6).Children(func() {
			kit.Label(c, fmt.Sprintf("FPS %.1f", latest.fps), kit.Strong)
			kit.Label(c, fmt.Sprintf("%.1f ms", latest.frame), kit.Caption).TextColor(kit.Text.Alpha(.72))
			kit.Label(c, map[bool]string{true: "GPU", false: "CPU"}[accelerated], kit.Caption).TextColor(kit.Text.Alpha(.72))
		})
		fpsChart(c, samples)
	})
}

func fpsChart(c *ui.Context, samples []fpsSample) {
	ui.Box(c).Grow(1).MinHeight(90).FillWidth().Background(kit.InsetColor).Border(1, kit.LineColor).Radius(kit.RadiusM).Padding(4).Draw(func(p *ui.Painter, r ui.Rect) {
		if r.W <= 0 || r.H <= 0 {
			return
		}
		const labelWidth, timeLabelHeight = float32(27), float32(15)
		plot := ui.Rect{X: r.X + labelWidth, Y: r.Y + 4, W: r.W - labelWidth, H: r.H - timeLabelHeight - 4}
		if plot.W <= 0 || plot.H <= 0 {
			return
		}
		minValue, maxValue := 0.0, 60.0
		if len(samples) > 0 {
			observedMin, observedMax := samples[0].fps, samples[0].fps
			for _, sample := range samples[1:] {
				observedMin = min(observedMin, sample.fps)
				observedMax = max(observedMax, sample.fps)
			}
			step := 5.0
			if observedMax-observedMin >= 40 {
				step = 10
			}
			minValue = max(0, math.Floor(observedMin/step)*step)
			maxValue = math.Ceil(observedMax/step) * step
			for maxValue-minValue < 4*step {
				if minValue >= step {
					minValue -= step
				}
				if maxValue-minValue < 4*step {
					maxValue += step
				}
			}
		}
		gridColor := kit.LineColor
		for i := 1; i < 4; i++ {
			y := plot.Y + plot.H*float32(i)/4
			p.Line(plot.X, y, plot.X+plot.W, y, 1, gridColor)
		}
		fontSize := kit.FontPx(kit.Caption)
		labelColor := kit.Text.Alpha(.68)
		for i := 0; i <= 4; i++ {
			value := minValue + (maxValue-minValue)*float64(4-i)/4
			y := plot.Y + plot.H*float32(i)/4
			if i == 0 {
				y += 1
			} else if i == 4 {
				y -= fontSize
			} else {
				y -= fontSize / 2
			}
			p.Text(r.X, y, fmt.Sprintf("%.0f", value), fontSize, labelColor)
		}
		if len(samples) == 0 {
			return
		}
		points := make([][2]float32, len(samples))
		valueRange := maxValue - minValue
		for i, sample := range samples {
			x := plot.X + plot.W*float32(i)/float32(max(1, len(samples)-1))
			y := plot.Y + plot.H - float32((sample.fps-minValue)/valueRange)*plot.H
			points[i] = [2]float32{x, y}
		}
		p.StrokePolyline(points, 3, kit.AccentText)
		for i, label := range []string{"-10s", "-5s", "now"} {
			x := plot.X + plot.W*float32(i)/2
			labelWidth := kit.TextWidth(c, label, kit.Caption)
			if i == 1 {
				x -= labelWidth / 2
			} else if i == 2 {
				x -= labelWidth
			}
			p.Text(x, plot.Y+plot.H+2, label, fontSize, labelColor)
		}
	})
}
