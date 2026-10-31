package page

import (
	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type JavaData struct {
	Java                            []java.Runtime
	JavaLoading                     bool
	RescanJava, BrowseJava, AddJava func()
}

func Java(c *ui.Context, cfg *config.Config, data JavaData, save func()) {
	ui.Column(c).FillWidth().Shrink(0).Gap(24).Children(func() { javaSettings(c, cfg, data, save) })
}

func javaSettings(c *ui.Context, cfg *config.Config, data JavaData, save func()) {
	if cfg.Java.Mode == "" {
		cfg.Java.Mode = cfg.Java.SelectionMode()
	}
	kit.FormGroup(c, locales.Runtime(), nil, func() {
		kit.FormRow(c, locales.SelectionMode(), "", func() {
			kit.SegmentedValues(c, &cfg.Java.Mode, []kit.SelectOption[string]{{Value: "auto", Label: locales.Automatic()}, {Value: "version", Label: locales.ByGameVersion()}, {Value: "custom", Label: locales.Custom()}}, save)
		})
		if cfg.Java.Mode == "custom" {
			kit.FoldMotion(ui.Column(c).FillWidth()).Children(func() {
				kit.Divider(c)
				kit.FormRow(c, locales.JavaPath(), "", func() {
					kit.Input(c, &cfg.Java.Path).Width(kit.W(240)).Height(kit.ControlH()).MinWidth(0).Label(locales.JavaPath()).OnChange(save)
					kit.IconButton(c, kit.Folder, false, data.BrowseJava).Label(locales.ChooseJavaExecutable()).Tooltip(locales.ChooseJavaExecutable()).Disabled(data.BrowseJava == nil)
				})
			})
		}
		kit.Divider(c)
		kit.FormRow(c, locales.SkipJavaCompatibilityChecks(), "", func() {
			kit.Switch(c, &cfg.Java.IgnoreCompatibility).Label(locales.SkipJavaCompatibilityChecks()).OnChange(save)
		})
	})
	kit.FormGroup(c, locales.DiscoveredJavaRuntimes(), func() {
		ui.Row(c).Gap(8).Children(func() {
			kit.Ghost(c, locales.AddJava(), data.AddJava).Height(kit.TabH()).Disabled(data.AddJava == nil)
			label := locales.Scan()
			if data.JavaLoading {
				label = locales.Scanning()
			}
			kit.Ghost(c, label, data.RescanJava).Height(kit.TabH()).Disabled(data.JavaLoading || data.RescanJava == nil)
		})
	}, func() {
		if len(data.Java) == 0 {
			ui.Row(c).FillWidth().Height(kit.RowDoubleH()).Gap(8).AlignItems(ui.Center).Children(func() {
				if data.JavaLoading {
					ui.Spinner(c).Size(16, 16)
					kit.Line(c, locales.Scanning2(), kit.Caption)
				} else {
					kit.Line(c, locales.NoJavaInstallationFoundSpecifyAPathManuallyOrEnableAutomaticDownloads(), kit.Caption)
				}
			})
		}
		for _, runtime := range data.Java {
			c.Key(runtime.Path)
			selected := cfg.Java.RuntimePath(runtime.Major) == runtime.Path
			use := func() {
				if cfg.Java.Mode == "version" {
					if cfg.Java.Versions == nil {
						cfg.Java.Versions = make(map[int]string)
					}
					cfg.Java.Versions[runtime.Major] = runtime.Path
				} else {
					cfg.Java.Mode, cfg.Java.Path = "custom", runtime.Path
				}
				save()
			}
			kit.ListRow(c, kit.Row{Title: runtime.Label(), Subtitle: kit.MiddleEllipsis(runtime.Path, 64), Lead: func() { kit.IconBadge(c, kit.Coffee, 32) }, OnClick: use}, func(bool) {
				if selected {
					kit.Badge(c, locales.Selected2(), kit.AccentTone)
				} else {
					kit.Button(c, locales.Use(), use).Height(kit.TabH())
				}
			}).Margin(0, -8).Tooltip(runtime.Path)
		}
	})
	kit.FormGroup(c, locales.DownloadsPage(), nil, func() {
		kit.FormRow(c, locales.AutomaticallyDownloadJava(), "", func() {
			kit.Switch(c, &cfg.Java.AutoDownload).Label(locales.AutomaticallyDownloadJava()).OnChange(save)
		})
	})
}
