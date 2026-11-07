package page

import (
	"fmt"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
)

type JavaData struct {
	Java                            []java.Runtime
	JavaLoading, JavaDownloading    bool
	JavaDownloadOpen                *bool
	DownloadRequest                 *java.DownloadRequest
	RescanJava, BrowseJava, AddJava func()
	CanDeleteJava                   func(java.Runtime) bool
	DeleteJava                      func(java.Runtime)
	DownloadJava                    func(java.DownloadRequest)
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
			kit.ListRow(c, kit.Row{Title: runtime.Label(), Subtitle: kit.MiddleEllipsis(runtime.Path, 64), Lead: func() { kit.IconBadge(c, kit.Coffee, 32) }}, func(bool) {
				ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
					if data.CanDeleteJava != nil && data.CanDeleteJava(runtime) && data.DeleteJava != nil {
						var open *bool
						button := kit.IconButton(c, kit.Trash, false, func() {
							if open != nil {
								*open = true
							}
						}).Label(locales.Delete()).Tooltip(locales.Delete()).Disabled(data.JavaDownloading)
						open = ui.Local(button, "java-delete-popup", func() bool { return false })
						ui.PopoverBase(c, button, open, func(panel ui.Element) {
							kit.PopupPanel(c, panel, 240, button).AttachTo(button, ui.AnchorBottomEnd, ui.AnchorTopEnd).Padding(12)
							panel.Children(func() {
								kit.Label(c, fmt.Sprintf(locales.ConfirmDeleteJavaRuntime(), runtime.Label()), kit.Body).Wrap().MaxLines(2)
								ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Children(func() {
									kit.Danger(c, locales.Delete(), func() {
										*open = false
										data.DeleteJava(runtime)
									})
								})
							})
						})
					}
					if selected {
						kit.Badge(c, locales.Selected2(), kit.AccentTone)
					} else {
						kit.Button(c, locales.Use(), use).Height(kit.TabH())
					}
				})
			}).Margin(0, -8)
		}
	})
	kit.FormGroup(c, locales.DownloadsPage(), nil, func() {
		kit.FormRow(c, locales.AutomaticallyDownloadJava(), "", func() {
			kit.Switch(c, &cfg.Java.AutoDownload).Label(locales.AutomaticallyDownloadJava()).OnChange(save)
		})
		if data.JavaDownloadOpen != nil && data.DownloadRequest != nil && data.DownloadJava != nil {
			kit.Divider(c)
			kit.FormRow(c, locales.ManualDownloadJava(), "", func() {
				kit.Ghost(c, locales.Select(), func() { *data.JavaDownloadOpen = true }).Height(kit.TabH()).Disabled(data.JavaDownloading)
			})
		}
	})
	if data.JavaDownloadOpen != nil && data.DownloadRequest != nil && data.DownloadJava != nil {
		javaDownloadDialog(c, data)
	}
}

func javaDownloadDialog(c *ui.Context, data JavaData) {
	close := func() { *data.JavaDownloadOpen = false }
	request := data.DownloadRequest
	if request.Distribution == "" {
		request.Distribution = java.DistributionMojang
	}
	info, ok := java.DistributionInfoFor(request.Distribution)
	if !ok {
		request.Distribution = java.DistributionMojang
		info, _ = java.DistributionInfoFor(request.Distribution)
	}
	if request.PackageType == "" {
		request.PackageType = java.PackageJRE
	}
	if !info.HasJRE && request.PackageType != java.PackageJDK {
		request.PackageType = java.PackageJDK
	}
	if request.Major == 0 {
		request.Major = 17
	}
	distributionLabel := info.Label
	packageLabel := "JRE"
	if request.PackageType == java.PackageJDK {
		packageLabel = "JDK"
	}
	versionLabel := fmt.Sprintf("Java %d", request.Major)
	kit.Dialog(c, data.JavaDownloadOpen, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, locales.DownloadJava(), close)
			distributions := make([]kit.SelectOption[java.Distribution], 0, len(java.SupportedDistributions()))
			for _, distribution := range java.SupportedDistributions() {
				distributions = append(distributions, kit.SelectOption[java.Distribution]{Value: distribution.Value, Label: distribution.Label})
			}
			kit.Select(c, &request.Distribution, distributionLabel, distributions, 160, 220, func() {
				if distribution, ok := java.DistributionInfoFor(request.Distribution); ok && !distribution.HasJRE {
					request.PackageType = java.PackageJDK
				} else if request.PackageType == "" {
					request.PackageType = java.PackageJRE
				}
			}).FillWidth().MinWidth(0).Label(locales.JavaDistribution()).Disabled(data.JavaDownloading)
			packages := make([]kit.SelectOption[java.PackageType], 0, 2)
			if distribution, ok := java.DistributionInfoFor(request.Distribution); ok && distribution.HasJRE {
				packages = append(packages, kit.SelectOption[java.PackageType]{Value: java.PackageJRE, Label: "JRE"})
			}
			packages = append(packages, kit.SelectOption[java.PackageType]{Value: java.PackageJDK, Label: "JDK"})
			kit.Select(c, &request.PackageType, packageLabel, packages, 160, 220, nil).FillWidth().MinWidth(0).Label(locales.JavaPackageType()).Disabled(data.JavaDownloading)
			majors := make([]kit.SelectOption[int], 0, len(java.SupportedMajors()))
			for _, major := range java.SupportedMajors() {
				majors = append(majors, kit.SelectOption[int]{Value: major, Label: fmt.Sprintf("Java %d", major)})
			}
			kit.Select(c, &request.Major, versionLabel, majors, 160, 220, nil).FillWidth().MinWidth(0).Label(locales.JavaVersion()).Disabled(data.JavaDownloading)
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.Cancel(), close).Disabled(data.JavaDownloading)
				label := locales.DownloadJava()
				if data.JavaDownloading {
					label = locales.Downloading()
				}
				kit.Primary(c, label, func() {
					*data.DownloadRequest = *request
					close()
					data.DownloadJava(*request)
				}).Disabled(data.JavaDownloading)
			})
		})
	})
}
