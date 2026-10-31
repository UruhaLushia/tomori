package app

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) logf(format string, args ...any) {
	a.logSource("", fmt.Sprintf(format, args...))
}

func (a *app) logSource(source, text string) {
	at := time.Now()
	if a.logFile != nil {
		a.logMu.Lock()
		if source == "" {
			_, _ = fmt.Fprintf(a.logFile, "%s %s\n", at.Format(time.RFC3339), text)
		} else {
			_, _ = fmt.Fprintf(a.logFile, "%s [%s] %s\n", at.Format(time.RFC3339), source, text)
		}
		a.logMu.Unlock()
	}
	text = shortLogText(text)
	line := page.FormatLogLine(at, text)
	if source != "" {
		line.Source = source + " / " + line.Source
	}
	if a.logWake == nil {
		a.appendLog(line)
		return
	}
	if a.queueLog(line) {
		a.logWake <- struct{}{}
	}
}

func shortLogText(text string) string {
	const limit = 4 << 10
	if len(text) > limit {
		end := limit
		for end > 0 && !utf8.RuneStart(text[end]) {
			end--
		}
		text = text[:end] + " …"
	}
	return text
}

func (a *app) appendLog(line page.LogLine) {
	appendBoundedLog(&a.logs, &a.logStart, line)
	a.logRevision++
}

func appendBoundedLog(lines *[]page.LogLine, start *int, line page.LogLine) {
	const limit = 2000
	if len(*lines) == limit {
		(*lines)[*start] = line
		*start = (*start + 1) % limit
		return
	}
	*lines = append(*lines, line)
}

func (a *app) queueLog(line page.LogLine) bool {
	a.logQueueMu.Lock()
	defer a.logQueueMu.Unlock()
	appendBoundedLog(&a.logPending, &a.logPendingStart, line)
	if a.logScheduled {
		return false
	}
	a.logScheduled = true
	return true
}

func (a *app) flushLogs() {
	a.logQueueMu.Lock()
	defer a.logQueueMu.Unlock()
	for i := range a.logPending {
		a.appendLog(a.logPending[(a.logPendingStart+i)%len(a.logPending)])
	}
	clear(a.logPending)
	a.logPending = a.logPending[:0]
	a.logPendingStart, a.logScheduled = 0, false
}

func (a *app) clearLogs() {
	a.logs, a.logStart = nil, 0
	a.logRevision++
	a.logQueueMu.Lock()
	clear(a.logPending)
	a.logPending = a.logPending[:0]
	a.logPendingStart = 0
	a.logQueueMu.Unlock()
}

func (a *app) startLogUpdates() {
	a.logWake = make(chan struct{}, 1)
	if a.logStore != nil {
		a.workers.Go(func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-a.ctx.Done():
					return
				case <-ticker.C:
					a.pruneLogs()
				}
			}
		})
	}
	a.workers.Go(func() {
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-a.logWake:
			}
			timer.Reset(50 * time.Millisecond)
			select {
			case <-a.ctx.Done():
				return
			case <-timer.C:
			}
			a.win.Update(func() {
				if a.ctx.Err() == nil {
					a.flushLogs()
				}
			})
		}
	})
}

func (a *app) logLimitChanged() {
	a.logStore.SetLimit(int64(a.cfg.Logging.MaxMiB) << 20)
	a.logLimitError = ""
	a.saveConfig()
	a.workers.Go(a.pruneLogs)
}

func (a *app) pruneLogs() {
	if err := a.logStore.Prune(); err != nil {
		a.win.Update(func() {
			if a.ctx.Err() == nil {
				a.logLimitError = locales.FailedToPruneLogs() + err.Error()
			}
		})
	}
}
