package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/diagnostic"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type logDiagnosticState struct {
	data               page.LogDiagnosticData
	request, operation uint64
	cancel             context.CancelFunc
}

func (a *app) logSnapshot() diagnostic.Report {
	lines, start := a.logs, a.logStart
	source, truncated := locales.LiveLog(), len(lines) == 2000
	if a.logHistory.data.File != "" {
		lines, start = a.logHistory.data.Lines, 0
		source, truncated = a.logHistory.data.File, a.logHistory.data.Truncated
	}
	report := diagnostic.Report{Created: time.Now(), Source: source, System: "Tomori 0.1.0 · " + runtime.GOOS + "/" + runtime.GOARCH, Truncated: truncated, Records: make([]diagnostic.Record, 0, len(lines))}
	for i := range lines {
		line := lines[(start+i)%len(lines)]
		report.Truncated = report.Truncated || strings.HasSuffix(line.Text, " …")
		report.Records = append(report.Records, diagnostic.Record{Clock: line.Clock, Level: line.Level, Source: line.Source, Message: line.Message})
	}
	return report
}

func (a *app) openLogDiagnostic() {
	if a.pathBusy || a.logHistory.data.Loading {
		return
	}
	a.closeLogDiagnostic()
	s := &a.logDiagnostic
	s.request++
	viewID := s.request
	directory, _ := mygo.App.Path(mygo.PathDownloads)
	if directory == "" {
		directory, _ = os.UserHomeDir()
	}
	s.data = page.LogDiagnosticData{Open: true, Loading: true, Directory: directory, Format: "report"}
	a.pathBusy = true
	input := a.logSnapshot()
	var secrets []string
	for _, account := range a.cfg.Accounts.Entries {
		secrets = append(secrets, account.AccessToken, account.RefreshToken, account.ClientToken, account.LoginName, account.UserID, account.UUID, account.XUID)
	}
	secrets = append(secrets, a.cfg.Download.CurseForgeKey, a.cfg.Download.Proxy.Password)
	secrets = append(secrets, a.cfg.Game.Execution.Secrets()...)
	for _, settings := range a.cfg.Instances {
		secrets = append(secrets, settings.Game.Execution.Secrets()...)
	}
	home, _ := os.UserHomeDir()
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	a.workers.Go(func() {
		defer cancel()
		report, err := diagnostic.Prepare(ctx, input, diagnostic.NewRedactor(home, secrets))
		a.win.Update(func() {
			s := &a.logDiagnostic
			if a.ctx.Err() != nil || s.request != viewID || !s.data.Open {
				return
			}
			s.data.Loading = false
			if err != nil {
				s.data.Error = err.Error()
				return
			}
			s.data.Report = report
		})
	})
}

func (a *app) closeLogDiagnostic() {
	s := &a.logDiagnostic
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.data.Busy {
		a.cancelOperation(s.operation)
	}
	s.request++
	s.data, s.operation = page.LogDiagnosticData{}, 0
	if a.directoryPicker.data.Open && a.directoryPicker.kind == pickerLogReport {
		a.closeDirectoryPicker()
	}
	a.pathBusy = a.pathDialogsOpen()
}

func (a *app) chooseLogReportDirectory(directory string, request uint64) {
	s := &a.logDiagnostic
	if !s.data.Open || s.data.Busy || s.request != request {
		return
	}
	s.data.Directory, s.data.Error = directory, ""
}

func (a *app) exportLogReport() {
	s := &a.logDiagnostic
	if !s.data.Open || s.data.Loading || s.data.Busy || s.data.Directory == "" || len(s.data.Report.Records) == 0 || a.tasksFull() {
		return
	}
	d, viewID := s.data, s.request
	ctx, token := a.beginTask(locales.ExportLogs(), "", "")
	if token == 0 {
		return
	}
	s.data.Busy, s.data.Error, s.operation = true, "", token
	service := a.newService()
	a.workers.Go(func() {
		defer service.Close()
		path, err := service.ExportLogReport(ctx, d.Directory, d.Report, d.Format == "log")
		a.win.Update(func() {
			s := &a.logDiagnostic
			if a.ctx.Err() != nil || !a.operationCurrent(token) || s.request != viewID || !s.data.Open {
				return
			}
			s.data.Busy = false
			if err != nil {
				if !operationCanceled(err) {
					s.data.Error = err.Error()
				}
				return
			}
			s.data.Result = path
		})
		a.finishOperation(token, err)
	})
}

func (a *app) logDiagnosticView(c *ui.Context) {
	page.LogDiagnostic(c, &a.logDiagnostic.data, page.LogDiagnosticActions{Close: a.closeLogDiagnostic, Export: a.exportLogReport, Browse: func() {
		if a.logDiagnostic.data.Open && !a.logDiagnostic.data.Busy {
			a.openDirectoryPicker(pickerLogReport, a.logDiagnostic.data.Directory)
		}
	}, Open: func() {
		if path := a.logDiagnostic.data.Result; path != "" {
			a.openDirectory(filepath.Dir(path))
		}
	}})
}
