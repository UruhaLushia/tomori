package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
)

var (
	Window        ui.Color
	Canvas        ui.Color
	Surface       ui.Color
	SurfaceHi     ui.Color
	LineColor     ui.Color
	Text          ui.Color
	Muted         ui.Color
	Accent        ui.Color
	AccentHi      ui.Color
	Green         ui.Color
	Red           ui.Color
	Warning       ui.Color
	AccentInk     ui.Color
	AccentText    ui.Color
	WarningSoft   ui.Color
	ChartMuted    ui.Color
	ProgressMuted ui.Color
	// Deprecated: use Accent for progress indicators.
	LaunchProgress ui.Color

	LineSoft       ui.Color
	Shadow         ui.Color
	AccentSoft     ui.Color
	AccentLine     ui.Color
	AccentDeep     ui.Color
	Faint          ui.Color
	Hover          ui.Color
	Press          ui.Color
	InsetColor     ui.Color
	BackdropStrong ui.Color
	ImageInk       = ui.RGB(255, 255, 255)
	RedSoft        ui.Color
	GreenSoft      ui.Color
	RadiusS        float32
	RadiusM        float32
	RadiusL        float32
	RadiusFull     float32
	Backdrop       ui.Color

	dark        bool
	cachedTheme *ui.Theme
	current     Design
)

type Palette struct {
	Window, Canvas, InsetColor, Surface, SurfaceHi, Line, LineSoft  ui.Color
	Text, Muted, Hover, Press, Faint                                ui.Color
	Accent, AccentHi, AccentInk, AccentText, AccentSoft             ui.Color
	AccentLine, AccentDeep, Green, Red, Warning                     ui.Color
	WarningSoft, ChartMuted, ProgressMuted, LaunchProgress          ui.Color
	RedSoft, GreenSoft, Shadow, Backdrop, BackdropStrong, Scrollbar ui.Color
}

type Metrics struct {
	Radius, Spacing float32
}

type Design struct {
	Dark    bool
	Palette Palette
	Metrics Metrics
}

func init() {
	Apply(false, ui.Hex(config.DefaultAccent))
}

func Apply(isDark bool, accent ui.Color) {
	if current.Dark == isDark && current.Palette.Accent == accent {
		return
	}
	ApplyDesign(DefaultDesign(isDark, accent))
}

func ApplyDesign(design Design) {
	if design.Metrics.Radius <= 0 {
		design.Metrics.Radius = 10
	}
	if design.Metrics.Spacing <= 0 {
		design.Metrics.Spacing = 4
	}
	if cachedTheme != nil && current == design {
		return
	}
	current, dark = design, design.Dark
	cachedTheme = nil
	p := design.Palette
	Window, Canvas, InsetColor, Surface, SurfaceHi = p.Window, p.Canvas, p.InsetColor, p.Surface, p.SurfaceHi
	LineColor, Text, Muted = p.Line, p.Text, p.Muted
	Accent, AccentHi, AccentInk, AccentSoft = p.Accent, p.AccentHi, p.AccentInk, p.AccentSoft
	AccentText = p.AccentText
	AccentLine, AccentDeep, Faint, Hover, Press = p.AccentLine, p.AccentDeep, p.Faint, p.Hover, p.Press
	Green, Red, Warning = p.Green, p.Red, p.Warning
	WarningSoft, ChartMuted, ProgressMuted, LaunchProgress = p.WarningSoft, p.ChartMuted, p.ProgressMuted, p.LaunchProgress
	RedSoft, GreenSoft = p.RedSoft, p.GreenSoft
	LineSoft, Shadow, Backdrop, BackdropStrong = p.LineSoft, p.Shadow, p.Backdrop, p.BackdropStrong
	RadiusS, RadiusM, RadiusL, RadiusFull = design.Metrics.Radius*.6, design.Metrics.Radius, design.Metrics.Radius*1.4, 999
}

func Current() Design { return current }

func ParseColor(value string) ui.Color { return ui.Hex(value) }

func DefaultDesign(isDark bool, accent ui.Color) Design {
	p := Palette{
		Window: ui.Hex("#f1ede6"), Canvas: ui.Hex("#fbfaf7"), InsetColor: ui.Hex("#f4f1ec"),
		Accent: accent, AccentHi: accent.Mix(ui.Hex("#ffffff"), .16),
		AccentSoft: accent.Alpha(.14), AccentLine: accent.Alpha(.30),
		AccentDeep: accent.Mix(ui.Hex("#211c16"), .20), Faint: ui.RGBA(0, 0, 0, .07),
		Hover: ui.RGBA(33, 28, 22, .045), Press: ui.RGBA(33, 28, 22, .08),
		Backdrop: ui.RGBA(0, 0, 0, .35), BackdropStrong: ui.RGBA(0, 0, 0, .56), Scrollbar: ui.RGBA(0, 0, 0, .14),
	}
	if isDark {
		p.Window = ui.Hex("#15110f")
		p.Canvas = ui.Hex("#0f0c0b")
		p.InsetColor = ui.Hex("#0d0b0a")
		p.Surface = ui.Hex("#1a1512")
		p.SurfaceHi = ui.Hex("#221b16")
		p.Line = ui.Hex("#2b221c")
		p.Text = ui.Hex("#f5efe6")
		p.Muted = ui.Hex("#9b8d7f")
		p.Green = ui.Hex("#79c07d")
		p.Red = ui.Hex("#e2665a")
		p.Warning = ui.Hex("#e4b665")
		p.LineSoft = ui.RGBA(255, 255, 255, 0.07)
		p.Shadow = ui.RGBA(0, 0, 0, 0.35)
		p.Hover = ui.RGBA(255, 255, 255, .05)
		p.Press = ui.RGBA(255, 255, 255, .09)
		p.Faint = p.Muted.Alpha(.5)
		p.Scrollbar = ui.RGBA(255, 255, 255, .14)
	} else {
		p.Surface = ui.Hex("#ffffff")
		p.SurfaceHi = ui.Hex("#efe9e0")
		p.Line = ui.Hex("#e6dfd5")
		p.Text = ui.Hex("#211c16")
		p.Muted = ui.Hex("#8b8177")
		p.Green = ui.Hex("#4d9a5b")
		p.Red = ui.Hex("#cf5346")
		p.Warning = ui.Hex("#96601e")
		p.LineSoft = ui.RGBA(0, 0, 0, 0.06)
		p.Shadow = ui.RGBA(64, 45, 26, 0.12)
	}
	p.AccentHi.A, p.AccentDeep.A = accent.A, accent.A
	surfaceAccent := accent.Over(p.Surface)
	p.AccentInk = accentInk(surfaceAccent)
	p.Faint = p.Muted.Alpha(.5)
	p.RedSoft, p.GreenSoft = p.Red.Alpha(.16), p.Green.Alpha(.16)
	p.WarningSoft = p.Warning.Alpha(.16)
	p.ChartMuted = p.Muted.Alpha(.35)
	p.ProgressMuted = p.Muted.Alpha(.65)
	p.LaunchProgress = p.AccentInk.Alpha(.35)
	p.AccentText = accentText(surfaceAccent, p.Surface, p.Text)
	return Design{Dark: isDark, Palette: p, Metrics: Metrics{Radius: 10, Spacing: 4}}
}

func Dark() bool { return dark }

func Theme() *ui.Theme {
	if cachedTheme != nil {
		return cachedTheme
	}
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
	} else {
		t = ui.LightTheme()
	}
	t.Background = Canvas
	t.Surface = Surface
	t.SurfaceHover = SurfaceHi
	t.SurfacePressed = SurfaceHi
	t.Border = LineColor
	t.Text = Text
	t.TextMuted = Muted
	t.Accent = Accent
	t.AccentHover = AccentHi
	t.AccentPressed = Accent
	t.AccentText = AccentInk
	t.Danger = Red
	t.Success = Green
	t.Warning = Warning
	t.Selection = AccentSoft
	t.Focus = Accent
	t.Scrollbar = current.Palette.Scrollbar
	t.Radius = current.Metrics.Radius
	t.Spacing = current.Metrics.Spacing
	t.FontSize = FontPx(Body)
	t.Font = interfaceFont
	cachedTheme = t
	return t
}
