package page

import (
	"fmt"

	"github.com/UruhaLushia/tomori/internal/diagnostic"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
)

type LogDiagnosticData struct {
	Open, Loading, Busy              bool
	Report                           diagnostic.Report
	Directory, Format, Error, Result string
}
type LogDiagnosticActions struct{ Close, Browse, Export, Open func() }

func LogDiagnostic(c *ui.Context, data *LogDiagnosticData, actions LogDiagnosticActions) {
	if !data.Open {
		return
	}
	c.Key("log-diagnostic")
	dialog := kit.Dialog(c, &data.Open, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(560).Children(func() {
			kit.DialogHeader(c, locales.LogDiagnostics(), actions.Close)
			if data.Loading {
				ui.Box(c).FillWidth().Grow(1).Center().Children(func() { ui.Spinner(c) })
			} else {
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					kit.Label(c, data.Report.Source, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					kit.Label(c, fmt.Sprintf(locales.Entries(), len(data.Report.Records)), kit.Body).TextColor(kit.Muted)
				})
				if data.Report.Truncated {
					kit.Label(c, locales.RecentRecordsOnly(), kit.Body).TextColor(kit.Muted)
				}
				ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
					ui.Column(c).FillWidth().Padding(0, kit.ScrollbarGutter, 4, 0).Gap(12).Children(func() {
						if len(data.Report.Findings) == 0 {
							kit.Label(c, locales.NoClearCauseIdentified(), kit.Body).TextColor(kit.Muted).Padding(16, 0)
						}
						for _, finding := range data.Report.Findings {
							c.Key(finding.ID)
							kit.Card(c, ui.Column(c).Padding(12).Gap(8)).Children(func() {
								kit.Label(c, finding.Title, kit.Body).Wrap()
								label := locales.ReportedByLog()
								if finding.Possible {
									label = locales.PossibleCause()
								}
								kit.Label(c, label, kit.Body).TextColor(kit.Muted)
								kit.Label(c, finding.Advice, kit.Body).Wrap()
								for i, evidence := range finding.Evidence {
									c.Key(i)
									kit.Label(c, evidence, kit.Mono).TextColor(kit.Muted).FillWidth().Wrap().Selectable()
								}
							})
						}
					})
				})
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			if data.Result != "" {
				kit.Label(c, data.Result, kit.Body).TextColor(kit.Muted).FillWidth().Wrap().MaxLines(2).Ellipsis("…")
				kit.DialogFooter(c, func() { kit.Primary(c, locales.OpenExportFolder(), actions.Open) })
				return
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				kit.Label(c, data.Directory, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…").Tooltip(data.Directory)
				kit.Button(c, locales.Browse(), actions.Browse).Disabled(data.Loading || data.Busy)
			})
			kit.DialogFooter(c, func() {
				label := locales.DiagnosticReport()
				if data.Format == "log" {
					label = locales.Logs()
				}
				kit.Select(c, &data.Format, label, []kit.SelectOption[string]{{Value: "report", Label: locales.DiagnosticReport()}, {Value: "log", Label: locales.Logs()}}, 140, 160, nil).Disabled(data.Loading || data.Busy).Label(locales.ExportFormat())
				if data.Busy {
					ui.Spinner(c).Size(16, 16)
				}
				kit.Primary(c, locales.Export(), actions.Export).Disabled(data.Loading || data.Busy || data.Directory == "" || len(data.Report.Records) == 0).Label(fmt.Sprintf(locales.Export2(), label))
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}
