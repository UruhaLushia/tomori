package app

import (
	"context"
	"time"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/launcher"
)

type logHistoryState struct {
	data        page.LogHistoryData
	request     uint64
	cancel      context.CancelFunc
	listID      uint64
	listCancel  context.CancelFunc
	initialized bool
}

func (a *app) releaseLogHistory() {
	a.closeLogDiagnostic()
	s := &a.logHistory
	if s.cancel != nil {
		s.cancel()
	}
	if s.listCancel != nil {
		s.listCancel()
	}
	s.request++
	s.listID++
	s.data, s.initialized = page.LogHistoryData{}, false
	a.pages.logs = page.Logs{}
}

func (a *app) refreshLogHistory() {
	s := &a.logHistory
	if s.listCancel != nil {
		s.listCancel()
	}
	s.initialized, s.data.Listing = true, true
	s.listID++
	token := s.listID
	ctx, cancel := context.WithCancel(a.ctx)
	s.listCancel = cancel
	a.workers.Go(func() {
		defer cancel()
		files, err := a.logStore.History(ctx)
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.logHistory
			if a.ctx.Err() != nil || token != s.listID {
				return
			}
			s.data.Listing = false
			if err != nil {
				s.data.Error = err.Error()
				return
			}
			s.data.Files, s.data.Error = files, ""
		})
	})
}

func (a *app) chooseLogHistory(name string) {
	s := &a.logHistory
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	token := s.request
	s.data.File, s.data.Lines, s.data.Error, s.data.Truncated = name, nil, "", false
	s.data.Loading = name != ""
	if name == "" {
		return
	}
	at := time.Now()
	for _, file := range s.data.Files {
		if file.Name == name {
			at = file.Modified
			break
		}
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	a.workers.Go(func() {
		defer cancel()
		raw, truncated, err := a.logStore.Tail(ctx, name)
		var lines []page.LogLine
		if err == nil {
			err = launcher.DecodeLogLines(ctx, raw, func(text string) {
				text = shortLogText(text)
				line := page.FormatStoredLogLine(at, text)
				lines = append(lines, line)
			})
		}
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.logHistory
			if a.ctx.Err() != nil || token != s.request {
				return
			}
			s.data.Loading, s.data.Lines, s.data.Truncated = false, lines, truncated
			s.data.Revision++
			if err != nil {
				s.data.Error = err.Error()
			}
		})
	})
}
