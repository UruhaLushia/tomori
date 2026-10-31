package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/locales"
)

type PackExportData struct {
	Open, Busy, Identifying  bool
	Key, Source, Error, Path string
	Request                  launcher.PackExport
	Bytes                    int64
	Files                    int
}

type PackExportActions struct{ Save, Close, Browse, Open func() }

func PackExport(c *ui.Context, data *PackExportData, actions PackExportActions) {
	c.Key("pack-export")
	dialog := kit.Dialog(c, &data.Open, kit.DialogL, func(back, panel ui.Element) {
		panel.Height(600)
		panel.Children(func() {
			kit.DialogHeader(c, locales.ExportModpack(), actions.Close)
			if data.Path != "" {
				ui.Column(c).Grow(1).FillWidth().Gap(16).Justify(ui.Center).Children(func() {
					kit.Label(c, locales.ExportComplete(), kit.Body)
					kit.Label(c, data.Path, kit.Body).TextColor(kit.Muted).Wrap().MaxLines(4).Ellipsis("…")
				})
				kit.DialogFooter(c, func() { kit.Primary(c, locales.OpenExportFolder(), actions.Open) })
				return
			}
			ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(12).Padding(0, kit.ScrollbarGutter, 0, 0).Children(func() {
					kit.Label(c, data.Source, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
					ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
						kit.Label(c, locales.Format(), kit.Body).TextColor(kit.Muted)
						label := "Modrinth · MRPACK"
						switch data.Request.Format {
						case "prism":
							label = "Prism / MultiMC · ZIP"
						case "curseforge":
							label = "CurseForge · ZIP"
						case "mcbbs":
							label = "MCBBS · ZIP"
						}
						kit.Select(c, &data.Request.Format, label, []kit.SelectOption[string]{{Value: "mrpack", Label: "Modrinth · MRPACK"}, {Value: "curseforge", Label: "CurseForge · ZIP"}, {Value: "mcbbs", Label: "MCBBS · ZIP"}, {Value: "prism", Label: "Prism / MultiMC · ZIP"}}, 260, 280, nil).Grow(1).MinWidth(0).Disabled(data.Busy).Label(locales.Format())
					})
					ui.Row(c).FillWidth().Gap(8).Children(func() {
						kit.Input(c, &data.Request.Name).Grow(1).MinWidth(0).Placeholder(locales.ModpackName()).Label(locales.ModpackName()).Disabled(data.Busy)
						kit.Input(c, &data.Request.Version).Width(kit.W(110)).Placeholder(locales.Version()).Label(locales.Version()).Disabled(data.Busy)
					})
					if data.Request.Format != "curseforge" {
						ui.TextInput(c.Key("description"), &data.Request.Description).FillWidth().Placeholder(locales.DescriptionOptional()).Label(locales.ModpackDescription()).Disabled(data.Busy)
					}
					ui.Row(c.Key("destination")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						kit.Label(c, locales.SaveTo(), kit.Body).TextColor(kit.Muted).Shrink(0)
						kit.Label(c, data.Request.Directory, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
						kit.Button(c, locales.Browse(), actions.Browse).Disabled(data.Busy)
					})
					ui.Row(c.Key("filename")).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						kit.Input(c, &data.Request.Filename).Grow(1).MinWidth(0).Placeholder(locales.Filename()).Label(locales.Filename()).Disabled(data.Busy)
						kit.Label(c, data.Request.Extension(), kit.Body).TextColor(kit.Muted).Shrink(0)
					})
					kit.Card(c, ui.Column(c.Key("contents")).Padding(12).Gap(12)).Children(func() {
						kit.Label(c, locales.Include(), kit.Body)
						for _, option := range []struct {
							label string
							value *bool
						}{{locales.WorldsAndBackups(), &data.Request.Worlds}, {locales.Screenshots(), &data.Request.Screenshots}, {locales.LogsAndCrashReports(), &data.Request.Logs}, {locales.OtherPersonalData(), &data.Request.Personal}} {
							c.Key(option.label)
							ui.Checkbox(c, option.value, option.label).Disabled(data.Busy)
						}
					})
					if data.Request.Format == "mrpack" || data.Request.Format == "curseforge" {
						ui.Checkbox(c.Key("references"), &data.Request.References, locales.PreferOnlineDownloadReferencesSmallerArchive()).Disabled(data.Busy)
					}
				})
			})
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Busy {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					status := fmt.Sprintf(locales.ProcessedFilesMiB(), data.Files, float64(data.Bytes)/(1<<20))
					if data.Identifying {
						status = locales.IdentifyingOnlineResources()
					}
					kit.Label(c, status, kit.Body).TextColor(kit.Muted).Grow(1).MaxLines(1).Ellipsis("…")
				})
			}
			kit.DialogFooter(c, func() {
				kit.Primary(c, locales.Export(), actions.Save).Disabled(data.Busy || strings.TrimSpace(data.Request.Name) == "" || strings.TrimSpace(data.Request.Version) == "" || strings.TrimSpace(data.Request.Filename) == "" || data.Request.Directory == "")
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
