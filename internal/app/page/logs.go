package page

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/logstore"
	"github.com/UruhaLushia/tomori/locales"
)

type Logs struct {
	list, historyList                    ui.ListState
	query, level                         string
	historyOpen                          bool
	live                                 int
	frozen                               []LogLine
	frozenRevision                       uint64
	filterQuery, filterLevel, filterFile string
	filterRevision                       uint64
	filterStart, filterLength            int
	filtered                             []int
}

type LogHistoryData struct {
	Files                       []logstore.HistoryFile
	File, Error                 string
	Lines                       []LogLine
	Loading, Listing, Truncated bool
	Revision                    uint64
}

type LogsData struct {
	Lines    []LogLine
	Start    int
	Revision uint64
	History  LogHistoryData
}

type LogsActions struct {
	Clear, Refresh, OpenDirectory func()
	Analyze, Tabs                 func()
	Choose                        func(string)
}

func (p *Logs) View(c *ui.Context, data LogsData, actions LogsActions) {
	lines, start, revision := data.Lines, data.Start, data.Revision
	if data.History.File != "" {
		lines, start, revision = data.History.Lines, 0, data.History.Revision
	} else if p.live == 1 {
		lines, start, revision = p.frozen, 0, p.frozenRevision
	}
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(kit.Gap(c)).Children(func() {
		title := locales.Logs()
		if actions.Tabs != nil {
			title = locales.More()
		}
		kit.PageHeader(c, title, "", actions.Tabs, func() {
			kit.Button(c, locales.History(), func() { p.historyOpen = true; actions.Refresh() })
			kit.Button(c, locales.Diagnose(), actions.Analyze).Disabled(data.History.Loading || len(lines) == 0)
			if !kit.Compact(c) {
				kit.Ghost(c, locales.ClearView(), actions.Clear).Disabled(data.History.File != "")
			} else {
				kit.MoreMenu(c, locales.MoreLogActions(), func(menu *kit.ContextActions) {
					menu.Item(c, "clear", locales.ClearView(), data.History.File != "", actions.Clear)
				})
			}
			kit.IconButton(c, kit.Folder, false, actions.OpenDirectory).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
		})
		kit.Toolbar(c, func() {
			kit.SearchInput(c, &p.query, locales.SearchLogs()).Grow(1).Basis(160)
			level := locales.AllLevels()
			if p.level != "" {
				level = p.level
			}
			kit.FilterSelect(c, &p.level, level, []kit.SelectOption[string]{
				{Value: "", Label: locales.AllLevels()}, {Value: "ERROR", Label: "ERROR / FATAL"},
				{Value: "WARN", Label: "WARN"}, {Value: "INFO", Label: "INFO"},
				{Value: "DEBUG", Label: "DEBUG"}, {Value: "TRACE", Label: "TRACE"},
			}, 160, nil)
			if data.History.File == "" {
				kit.Segmented(c, &p.live, locales.Live(), locales.Pause()).OnChange(func() {
					if p.live == 1 {
						p.frozen = make([]LogLine, len(data.Lines))
						for i := range data.Lines {
							p.frozen[i] = data.Lines[(data.Start+i)%len(data.Lines)]
						}
						p.frozenRevision = data.Revision
					} else {
						p.frozen = nil
					}
					p.filterLength = -1
				})
			} else {
				kit.Line(c, data.History.File, kit.Caption).MaxWidth(200)
				kit.Ghost(c, locales.BackToLive(), func() { p.live = 0; p.frozen = nil; actions.Choose("") })
			}
		})
		if data.History.Error != "" {
			kit.ErrorText(c, data.History.Error)
		}
		p.filter(lines, start, revision, data.History.File)
		p.list.FollowEnd = data.History.File == "" && p.live == 0
		p.list.FollowAnimation = &kit.MotionFast
		kit.Inset(c, ui.Column(c)).Grow(1).FillWidth().MinHeight(0).Margin(0, 0, kit.Pad(c), 0).Children(func() {
			if data.History.Loading {
				kit.Loading(c)
				return
			}
			if len(p.filtered) == 0 {
				kit.EmptyState(c, kit.List, locales.NoMatchingLogs(), locales.TryAnotherLevelOrSearchTerm(), nil)
				return
			}
			c.Key("log-results")
			ui.List(c, &p.list, len(p.filtered)).Grow(1).FillWidth().MinHeight(0).Padding(8, 12).
				ItemKey(func(i int) any { return start + p.filtered[i] }).Scrollbars(false).Rows(func(item ui.ListRow) {
				c := item.Context
				l := lines[(start+p.filtered[item.Index])%len(lines)]
				row := ui.Row(c).FillWidth().MinHeight(kit.H(20, kit.Mono)).Gap(12).AlignItems(ui.Start)
				if l.Level == "ERROR" || l.Level == "FATAL" {
					row.Background(kit.RedSoft)
				}
				row.Children(func() {
					kit.Line(c, l.Clock, kit.Mono).Width(64).Shrink(0).TextColor(kit.Muted)
					kit.LogLevel(c, l.Level, logColor(l.Level)).Width(40).Shrink(0)
					kit.Line(c, l.Source, kit.Mono).Width(96).Shrink(0).TextColor(kit.Muted)
					kit.Label(c, l.Message, kit.Mono).Grow(1).Basis(0).MinWidth(0).Wrap().Selectable()
				})
				services := c.Services()
				kit.ContextMenu(c, row, func(menu *kit.ContextActions) {
					menu.Item(c, "copy", locales.Copy(), false, func() { services.WriteClipboard(l.Clock + " [" + l.Level + "] " + l.Source + " · " + l.Message) })
				})
			})
		})
	})
	p.history(c, data.History, actions)
}

func (p *Logs) filter(lines []LogLine, start int, revision uint64, file string) {
	if p.filterRevision == revision && p.filterStart == start && p.filterLength == len(lines) && p.filterQuery == p.query && p.filterLevel == p.level && p.filterFile == file {
		return
	}
	if p.filterQuery != p.query || p.filterLevel != p.level || p.filterFile != file {
		p.list = ui.ListState{}
	}
	p.filterQuery, p.filterLevel, p.filterFile = p.query, p.level, file
	p.filterRevision, p.filterStart, p.filterLength = revision, start, len(lines)
	p.filtered = p.filtered[:0]
	query := []rune(strings.ToLower(strings.TrimSpace(p.query)))
	for i := range lines {
		line := lines[(start+i)%len(lines)]
		if p.level != "" && p.level != line.Level && (p.level != "ERROR" || line.Level != "FATAL") {
			continue
		}
		if len(query) != 0 && !containsLogText(line.Message, query) && !containsLogText(line.Source, query) {
			continue
		}
		p.filtered = append(p.filtered, i)
	}
}

func containsLogText(text string, query []rune) bool {
	for i, r := range text {
		if unicode.ToLower(r) != query[0] {
			continue
		}
		tail, matched := text[i:], true
		for _, expected := range query {
			r, size := utf8.DecodeRuneInString(tail)
			if size == 0 || unicode.ToLower(r) != expected {
				matched = false
				break
			}
			tail = tail[size:]
		}
		if matched {
			return true
		}
	}
	return false
}

func (p *Logs) history(c *ui.Context, data LogHistoryData, actions LogsActions) {
	if !p.historyOpen {
		return
	}
	dialog := kit.Dialog(c, &p.historyOpen, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(420).Children(func() {
			kit.DialogHeader(c, locales.LogHistory(), func() { p.historyOpen = false })
			if data.Listing {
				ui.Box(c).FillWidth().Grow(1).Center().Children(func() { ui.Spinner(c) })
			} else {
				ui.List(c, &p.historyList, len(data.Files)).Grow(1).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0).
					ItemKey(func(i int) any { return data.Files[i].Name }).Scrollbars(false).Rows(func(row ui.ListRow) {
					c, file := row.Context, data.Files[row.Index]
					button := kit.RowButton(c, "", false, func() { p.historyOpen = false; actions.Choose(file.Name) }).FillWidth().Padding(12, 8)
					button.Children(func() {
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							kit.Line(c, file.Name, kit.Caption)
							kit.Label(c, fmt.Sprintf("%s · %.1f MiB", file.Modified.Format("2006-01-02 15:04"), float64(file.Size)/(1<<20)), kit.Caption)
						})
						ui.Icon(c, kit.ChevronRight).FontSize(kit.IconPx(12)).TextColor(kit.Muted)
					})
				})
			}
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			kit.DialogFooter(c, func() { kit.Button(c, locales.Refresh(), actions.Refresh).Disabled(data.Listing) })
		})
	})
	if dialog.Dismissed() {
		p.historyOpen = false
	}
}

func logColor(level string) ui.Color {
	switch level {
	case "ERROR", "FATAL":
		return kit.Red
	case "WARN":
		return kit.Warning
	case "DEBUG", "TRACE":
		return kit.Muted
	}
	return kit.Muted
}
