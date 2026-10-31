package page

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type Appearance struct {
	accent        ui.Color
	accentModel   string
	accentOpen    bool
	font          fontChoice
	logFont       fontChoice
	zoomModel     int
	zoomIndex     int
	pendingWeight int
	weightModel   int
	pendingScale  int
	scaleModel    int
	scaleInput    ui.Handle
	scaleFocused  bool
	scaleCommit   time.Time
}

func resetButtonWidth(c *ui.Context) float32 {
	return max(kit.W(96), kit.TextWidth(c, locales.RestoreDefault(), kit.Body)+kit.W(16))
}

func (p *Appearance) View(c *ui.Context, cfg *config.Appearance, fonts []string, currentZoom, maxZoom int, previewZoom, zoom func(int), save, gpuChanged, fpsChanged func()) {
	if p.accentModel != cfg.Accent {
		p.accent = kit.ParseColor(cfg.Accent)
		p.accentModel = cfg.Accent
	}
	ui.Column(c).FillWidth().Shrink(0).Gap(kit.S24).Children(func() {
		kit.Group(c, locales.Theme(), func() {
			kit.SettingRow(c, locales.AccentColor(), "", func() {
				p.accentPicker(c, cfg, save)
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.ColorMode(), "", func() {
				kit.Segmented(c, &cfg.ColorMode, locales.System(), locales.Light(), locales.Dark()).OnChange(save)
			})
		})
		kit.Group(c, locales.Layout(), func() {
			kit.SettingRow(c, locales.Navigation(), "", func() {
				kit.Segmented(c, &cfg.Navigation, locales.Sidebar(), locales.TopBar(), locales.IconsOnly()).OnChange(save)
			})
			kit.Divider(c)
			p.interfaceZoom(c, currentZoom, maxZoom, previewZoom, zoom)
		})
		kit.Group(c, locales.Rendering(), func() {
			kit.SettingRow(c, locales.GpuAcceleration(), "", func() {
				kit.Segmented(c, &cfg.GPU, locales.Auto(), locales.ForceGpu(), locales.ForceCpu()).OnChange(gpuChanged)
			})
		})
		kit.Group(c, locales.Debug(), func() {
			kit.SettingRow(c, locales.ShowFps(), "", func() {
				kit.Switch(c, &cfg.ShowFPS).Label(locales.ShowFps()).OnChange(fpsChanged)
			})
		})
		kit.Group(c, locales.Fonts(), func() {
			kit.SettingRow(c, locales.UiFont(), "", func() {
				p.font.input(c, &cfg.Font, fonts, locales.UiFont(), save)
			})
			kit.Divider(c)
			p.fontWeight(c, cfg, save)
			kit.Divider(c)
			p.fontScale(c, cfg, save)
			kit.Divider(c)
			kit.FormWideRow(c, "", "", func() {
				ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
					kit.Line(c, locales.LogFont(), kit.Body).Grow(1)
					p.logFont.input(c, &cfg.LogFont, fonts, locales.LogFont(), save)
				})
				kit.Inset(c, ui.Box(c)).FillWidth().Padding(8, 12).Children(func() { kit.Line(c, "[12:34:56] [INFO] Preparing spawn area: 100%", kit.Mono).TextColor(kit.Muted) })
			})
		})
	})
}

func (p *Appearance) CancelZoomPreview() {
	p.zoomIndex = max(0, slices.Index(config.ZoomSteps[:], p.zoomModel))
}

func (p *Appearance) interfaceZoom(c *ui.Context, current, maximum int, preview, apply func(int)) {
	current = min(current, maximum)
	last := max(0, slices.Index(config.ZoomSteps[:], maximum))
	if p.zoomModel != current {
		p.zoomModel = current
		p.CancelZoomPreview()
	}
	p.zoomIndex = max(0, min(last, p.zoomIndex))
	selected := config.ZoomSteps[p.zoomIndex]
	stage := func(index int) {
		p.zoomIndex = max(0, min(last, index))
		value := config.ZoomSteps[p.zoomIndex]
		if value == current {
			value = 0
		}
		preview(value)
	}
	commit := func(index int) {
		p.zoomIndex = max(0, min(last, index))
		preview(0)
		apply(config.ZoomSteps[p.zoomIndex])
	}
	ui.Row(c).FillWidth().MinHeight(kit.H(52, kit.Body)).Padding(12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		kit.Line(c, locales.InterfaceScale(), kit.Body).Grow(1).Basis(0).MinWidth(kit.W(120))
		ui.Row(c).Width(kit.W(420)).MinWidth(kit.W(252)).MaxWidthPercent(100).Shrink(1).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			ui.Box(c).Width(resetButtonWidth(c)).Height(kit.TabH()).Children(func() {
				if selected != min(100, maximum) {
					kit.Ghost(c, locales.RestoreDefault(), func() { commit(slices.Index(config.ZoomSteps[:], min(100, maximum))) }).Height(kit.TabH())
				}
			})
			c.Key("interface-zoom")
			slider := kit.StepSlider(c, p.zoomIndex, 0, max(1, last), 1, min(last, slices.Index(config.ZoomSteps[:], 100)), stage, commit).
				Label(locales.InterfaceScale()).Value(fmt.Sprintf("%d%%", selected)).Range(75, float64(maximum), float64(selected)).Disabled(last == 0)
			slider.Grow(1).MinWidth(kit.W(80)).MaxWidth(kit.W(240))
			ui.Row(c).Width(kit.W(88)).Height(kit.ControlH()).Padding(0, 10).Gap(8).Shrink(0).Radius(kit.RadiusS).Background(kit.InsetColor).AlignItems(ui.Center).Children(func() {
				kit.Line(c, fmt.Sprint(selected), kit.Strong).Grow(1)
				kit.Label(c, "%", kit.Caption)
			})
		})
	})
}

func (p *Appearance) fontWeight(c *ui.Context, cfg *config.Appearance, save func()) {
	model := cfg.FontWeight
	if model == 0 {
		model = config.DefaultFontWeight
	}
	if p.weightModel != model {
		p.weightModel, p.pendingWeight = model, model
	}
	preview := func(v int) { p.pendingWeight = v }
	commit := func(v int) {
		v = max(100, min(900, int(math.Round(float64(v)/100))*100))
		preview(v)
		if cfg.FontWeight != v {
			cfg.FontWeight, p.weightModel = v, v
			save()
		}
	}
	ui.Row(c).FillWidth().MinHeight(kit.H(52, kit.Body)).Padding(12, 0).Gap(16).AlignItems(ui.Center).Children(func() {
		kit.Line(c, locales.FontWeight(), kit.Body).Grow(1).Basis(0).MinWidth(kit.W(120))
		ui.Row(c).Width(kit.W(420)).MinWidth(kit.W(252)).MaxWidthPercent(100).Shrink(1).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
			ui.Box(c).Width(resetButtonWidth(c)).Height(kit.TabH()).Children(func() {
				if p.pendingWeight != config.DefaultFontWeight {
					kit.Ghost(c, locales.RestoreDefault(), func() { commit(config.DefaultFontWeight) }).Height(kit.TabH())
				}
			})
			c.Key("font-weight")
			kit.StepSlider(c, p.pendingWeight, 100, 900, 100, config.DefaultFontWeight, preview, commit).
				Label(locales.FontWeight()).Grow(1).MinWidth(kit.W(80)).MaxWidth(kit.W(240))
			c.Key("font-weight-value")
			kit.NumberField(c, &p.pendingWeight, 100, 900, 100, "", locales.FontWeightValue(), func() { commit(p.pendingWeight) })
		})
	})
}

func (p *Appearance) fontScale(c *ui.Context, cfg *config.Appearance, save func()) {
	model := cfg.FontScale
	if model == 0 {
		model = 100
	}
	if p.scaleModel != model {
		p.scaleModel, p.pendingScale = model, model
	}
	preview := func(v int) { p.pendingScale = v }
	commit := func(v int) {
		p.scaleCommit = time.Time{}
		v = max(75, min(config.MaxFontScale, int(math.Round(float64(v)/5))*5))
		preview(v)
		if cfg.FontScale != v {
			cfg.FontScale, p.scaleModel = v, v
			save()
		}
	}
	if !p.scaleCommit.IsZero() {
		if !c.Now().Before(p.scaleCommit) {
			commit(p.pendingScale)
		} else {
			c.After(p.scaleCommit.Sub(c.Now()))
		}
	}
	ui.Column(c).FillWidth().Padding(12, 0).Gap(8).Children(func() {
		controls := func() {
			ui.Row(c).Width(kit.W(420)).MinWidth(kit.W(252)).MaxWidthPercent(100).Shrink(1).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				// Keep this slot fixed so changing the scale never shifts the slider or input.
				ui.Box(c).Width(resetButtonWidth(c)).Height(kit.TabH()).Children(func() {
					if p.pendingScale != 100 {
						kit.Ghost(c, locales.RestoreDefault(), func() { commit(100) }).Height(kit.TabH())
					}
				})
				c.Key("font-scale")
				slider := kit.StepSlider(c, p.pendingScale, 75, config.MaxFontScale, 5, 100, preview, commit).Label(locales.FontSize())
				slider.Grow(1).MinWidth(kit.W(80)).MaxWidth(kit.W(240))
				c.Key("font-scale-value")
				input := kit.NumberField(c, &p.pendingScale, 75, config.MaxFontScale, 5, "%", locales.FontScalePercentage(), func() {
					if p.pendingScale != cfg.FontScale {
						p.scaleCommit = c.Now().Add(300 * time.Millisecond)
					}
				}).Bind(&p.scaleInput).OnSubmit(func() { commit(p.pendingScale) })
				if p.scaleFocused && !p.scaleInput.Focused(c) {
					commit(p.pendingScale)
				}
				p.scaleFocused = input.Focused()
			})
		}
		ui.Row(c).FillWidth().MinHeight(kit.H(52, kit.Body)).Padding(8, 0).Gap(16).AlignItems(ui.Center).Children(func() {
			kit.Line(c, locales.FontSize(), kit.Body).Grow(1).Basis(0).MinWidth(kit.W(120))
			controls()
		})
		kit.Inset(c, ui.Column(c)).FillWidth().Padding(8, 16).Gap(4).Children(func() {
			kit.Line(c, fmt.Sprintf(locales.Preview2(), p.pendingScale), kit.SectionText)
			kit.WithFontScale(c, p.pendingScale, func() {
				kit.ListRow(c, kit.Row{Title: locales.NewWorld(), Subtitle: locales.Value1211Survival(), Lead: func() { kit.IconBadge(c, kit.Folder, 32) }}, nil)
			})
		})
	})
}

func (p *Appearance) accentPicker(c *ui.Context, cfg *config.Appearance, save func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		preset := false
		for _, value := range kit.AccentPresets {
			c.Key(value)
			selected := kit.ParseColor(value) == p.accent
			preset = preset || selected
			kit.ColorSwatch(c, kit.ParseColor(value), selected, func() { cfg.Accent = value; p.accent = kit.ParseColor(value); p.accentModel = value; save() })
		}
		color := p.accent
		if preset {
			color = kit.ProgressMuted
		}
		button := kit.ColorSwatch(c, color, !preset, func() { p.accentOpen = !p.accentOpen }).Label(locales.CustomAccentColor()).Expanded(p.accentOpen)
		button.Center().Children(func() { ui.Icon(c, kit.Plus).FontSize(kit.IconPx(12)).TextColor(kit.Surface) })
		ui.PopoverBase(c, button, &p.accentOpen, func(panel ui.Element) {
			kit.PopupPanel(c, panel, 240).AttachTo(button, ui.AnchorBottomEnd, ui.AnchorTopEnd).Padding(12)
			ui.ColorPicker(c, &p.accent).FillWidth().MinWidth(0).OnChange(func() { cfg.Accent = kit.HexColor(p.accent); p.accentModel = cfg.Accent; save() })
		})
	})
}

type fontChoice struct {
	name    string
	model   string
	ready   bool
	open    bool
	locale  string
	source  *string
	count   int
	options []string
}

func (f *fontChoice) input(c *ui.Context, value *string, fonts []string, label string, save func()) {
	var source *string
	if len(fonts) > 0 {
		source = &fonts[0]
	}
	if !f.ready || f.model != *value || f.locale != i18n.Locale() || f.source != source || f.count != len(fonts) {
		f.locale, f.source, f.count = i18n.Locale(), source, len(fonts)
		f.model, f.name, f.ready = *value, *value, true
		if f.name == "" {
			f.name = locales.Default()
		}
		f.options = append(f.options[:0], locales.Default())
		f.options = append(f.options, fonts...)
		if !slices.Contains(f.options, f.name) {
			f.options = append(f.options, f.name)
		}
	}
	c.Key(label)
	parts := ui.ComboboxBase(c, &f.name)
	input := parts.Input.Width(kit.W(240)).MaxWidthPercent(100).Height(kit.ControlH()).MinWidth(0).SingleLine().Tooltip(f.name).Radius(kit.RadiusS).
		Background(kit.InsetColor).TextColor(kit.Text).FontSize(kit.FontPx(kit.Body)).
		Border(1, kit.LineColor).Padding(0, 10).PaddingEnd(38).Label(label)
	input.DrawOver(func(p *ui.Painter, r ui.Rect) {
		size := kit.IconPx(12)
		endWidth := kit.W(36)
		// Keep long font names from painting over the dropdown affordance.
		p.Fill(ui.Rect{X: r.X + r.W - endWidth, Y: r.Y + 1, W: endWidth - 1, H: r.H - 2}, kit.InsetColor, 0)
		p.Icon(kit.ChevronDown, ui.Rect{X: r.X + r.W - size - kit.W(10), Y: r.Y + (r.H-size)/2, W: size, H: size}, kit.Muted)
	})
	if input.FocusVisible() {
		kit.FocusRing(input, kit.RadiusS)
	}
	wasOpen := f.open
	shown := f.options
	if parts.Filtering() {
		query := strings.ToLower(strings.TrimSpace(f.name))
		var starts, contains []string
		for _, option := range f.options {
			lower := strings.ToLower(option)
			switch {
			case strings.HasPrefix(lower, query):
				starts = append(starts, option)
			case strings.Contains(lower, query):
				contains = append(contains, option)
			}
		}
		shown = append(starts, contains...)
	}
	parts.Popup(func(panel ui.Element) {
		kit.PopupPanel(c, panel, kit.W(200))
		panel.MaxHeight(kit.H(360))
		ui.Scroll(c).Scrollbars(false).Fill().MinHeight(0).Background(kit.Surface).Children(func() {
			ui.Column(c).FillWidth().Background(kit.Surface).Gap(2).Children(func() {
				for _, option := range shown {
					c.Key(option)
					item := parts.Item(option).FillWidth().Height(kit.ControlH()).Padding(0, 8).Radius(kit.RadiusS).AlignItems(ui.Center).Label(option)
					bg := kit.Surface
					if item.Highlighted() || item.Hovered() {
						bg = kit.Hover
					}
					item.Background(bg).Children(func() {
						kit.Line(c, option, kit.Body).Grow(1).Shrink(1).Tooltip(option)
					})
				}
			})
		})
	})
	selected, chosen := parts.Chosen()
	if !wasOpen && parts.Open() && !parts.Filtering() {
		input.SetTextSelection(0, len([]rune(f.name)))
	}
	f.open = parts.Open()
	if chosen {
		f.name = selected
		*value = selected
		if selected == locales.Default() {
			*value = ""
		}
		f.model = *value
		save()
	} else {
		selected := *value
		if selected == "" {
			selected = locales.Default()
		}
		if !input.Focused() && !parts.Open() && f.name != selected {
			f.name = selected
		}
	}
}
