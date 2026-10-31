package app

import (
	"context"
	"os"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/launcher"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type homeState struct {
	recent                          []page.HomeRecent
	recentScratch                   []page.HomeRecent
	history                         playHistorySummary
	loadedAt                        time.Time
	source                          string
	cancel                          context.CancelFunc
	request                         uint64
	metricsAt                       time.Time
	metricsBusy                     bool
	cpu                             float64
	cpuReady                        bool
	launcherCPU                     float64
	launcherMiB                     int
	checkedAt                       time.Time
	mods                            map[string]int
	gameCPU                         float64
	gameMiB                         int
	gameProbe, launcherProbe        processProbe
	failureKey, failure, failureLog string
	failureAction                   string
}

type playHistoryKey struct {
	instance string
	started  int64
}

type playHistorySession struct {
	seconds int64
	count   int
	days    [7]float64
}

type playHistorySummary struct {
	valid        bool
	day          time.Time
	first        time.Time
	validUntil   time.Time
	days         map[string][7]float64
	launches     map[string]int
	lastPlayed   map[string]time.Time
	totals       map[string]int64
	sessions     map[playHistoryKey]playHistorySession
	recentServer []config.PlayRecord
}

func (a *app) playHistory(now time.Time) *playHistorySummary {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	history := &a.home.history
	if history.valid && history.day.Equal(today) && (history.validUntil.IsZero() || now.Before(history.validUntil)) {
		return history
	}
	activeSessions := make(map[playHistoryKey]struct{}, len(a.games))
	for key, session := range a.games {
		if session != nil && !session.playedAt.IsZero() {
			activeSessions[playHistoryKey{instance: key, started: session.playedAt.UnixNano()}] = struct{}{}
		}
	}
	*history = playHistorySummary{
		valid:      true,
		day:        today,
		first:      today.AddDate(0, 0, -6),
		validUntil: today.AddDate(0, 0, 1),
		days:       make(map[string][7]float64),
		launches:   make(map[string]int),
		lastPlayed: make(map[string]time.Time),
		totals:     make(map[string]int64),
		sessions:   make(map[playHistoryKey]playHistorySession, len(activeSessions)),
	}
	for _, record := range a.cfg.PlayHistory {
		if record.Started.After(now) && record.Started.Before(history.validUntil) {
			history.validUntil = record.Started
		}
		if record.Started.After(history.lastPlayed[record.Instance]) {
			history.lastPlayed[record.Instance] = record.Started
		}
		seconds := max(int64(0), record.Seconds)
		history.totals[record.Instance] += seconds
		id := playHistoryKey{instance: record.Instance, started: record.Started.UnixNano()}
		session, tracking := history.sessions[id]
		if _, active := activeSessions[id]; active {
			session.seconds += seconds
			session.count++
			tracking = true
		}
		if record.Started.After(history.first) && record.Started.Before(now) {
			history.launches[record.Instance]++
		}
		recordDays := [7]float64{}
		addPlayDays(&recordDays, history.first, record.Started, seconds, 1)
		days := history.days[record.Instance]
		for i, value := range recordDays {
			days[i] += value
		}
		history.days[record.Instance] = days
		if tracking {
			for i, value := range recordDays {
				session.days[i] += value
			}
			history.sessions[id] = session
		}
	}
	seen := make(map[string]struct{}, 8)
	for i := len(a.cfg.PlayHistory) - 1; i >= 0; i-- {
		record := a.cfg.PlayHistory[i]
		id := record.Instance + ":" + record.Server
		if record.Server == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		history.recentServer = append(history.recentServer, record)
		if len(seen) >= 8 {
			break
		}
	}
	return history
}

func addPlayDays(days *[7]float64, first, started time.Time, seconds int64, scale float64) {
	if seconds <= 0 {
		return
	}
	end := started.Add(time.Duration(seconds) * time.Second)
	for i := range days {
		day := first.AddDate(0, 0, i)
		start, stop := started, end
		if start.Before(day) {
			start = day
		}
		if next := day.AddDate(0, 0, 1); stop.After(next) {
			stop = next
		}
		if stop.After(start) {
			days[i] += stop.Sub(start).Seconds() * scale
		}
	}
}

func (a *app) invalidatePlayHistory() {
	a.home.history.valid = false
}

func (a *app) refreshHome() {
	if a.win == nil || a.ctx.Err() != nil {
		return
	}
	now := time.Now()
	if now.Sub(a.home.checkedAt) < 3*time.Second {
		return
	}
	a.home.checkedAt = now
	if a.page == pageHome && !a.home.metricsBusy && now.Sub(a.home.metricsAt) >= 3*time.Second {
		a.home.metricsBusy = true
		var pids []int
		for _, session := range a.games {
			if session.pid > 0 && session.activity.Running {
				pids = append(pids, session.pid)
			}
		}
		a.workers.Go(func() {
			values, err := cpu.PercentWithContext(a.ctx, 0, false)
			gameCPU, gameMiB := a.home.gameProbe.sample(a.ctx, pids, true)
			launcherCPU, launcherMiB := a.home.launcherProbe.sample(a.ctx, []int{os.Getpid()}, false)
			a.win.Update(func() {
				a.home.metricsBusy, a.home.metricsAt = false, time.Now()
				if a.ctx.Err() != nil {
					return
				}
				a.home.cpuReady = err == nil && len(values) > 0
				if a.home.cpuReady {
					a.home.cpu = values[0]
				}
				a.home.launcherCPU, a.home.launcherMiB = launcherCPU, launcherMiB
				a.home.gameCPU, a.home.gameMiB = gameCPU, gameMiB
			})
		})
	}
	var source strings.Builder
	roots := make([]string, len(a.installedList))
	for i, v := range a.installedList {
		roots[i] = v.ContentDirectory(a.cfg.GameForInstance(v.Key()).Isolate)
		source.WriteString(v.Key())
		source.WriteString("\x00")
		source.WriteString(roots[i])
		source.WriteString("\x00")
	}
	stamp := source.String()
	if a.home.source == stamp && (a.home.cancel != nil || now.Sub(a.home.loadedAt) < 30*time.Second) {
		return
	}
	a.cancelHomeScan()
	ctx, cancel := context.WithCancel(a.ctx)
	a.home.source, a.home.cancel = stamp, cancel
	request := a.home.request
	instances := slices.Clone(a.installedList)
	a.workers.Go(func() {
		var recent []page.HomeRecent
		mods := make(map[string]int)
		rootMods := make(map[string]int)
		seen := make(map[string]bool)
		for i, v := range instances {
			if ctx.Err() != nil {
				break
			}
			if seen[roots[i]] {
				mods[v.Key()] = rootMods[roots[i]]
				continue
			}
			seen[roots[i]] = true
			summary, _ := instancecontent.Scan(ctx, roots[i])
			mods[v.Key()] = summary.Resources[resource.Mod].Total
			rootMods[roots[i]] = mods[v.Key()]
			for _, file := range summary.Worlds[:min(5, len(summary.Worlds))] {
				world, _ := instancecontent.ReadWorld(ctx, file.Path)
				name, detail, played := file.Name, v.GameVersion, file.Modified
				if world.Name != "" {
					name = world.Name
				}
				if world.Valid {
					detail = kit.MetaLine(world.Version, worldModeLabel(world))
				}
				if !world.LastPlayed.IsZero() {
					played = world.LastPlayed
				}
				recent = append(recent, page.HomeRecent{Instance: v, Name: name, Detail: detail, Path: file.Path, Played: played})
			}
		}
		slices.SortFunc(recent, func(a, b page.HomeRecent) int { return b.Played.Compare(a.Played) })
		recent = recent[:min(12, len(recent))]
		a.win.Update(func() {
			// Update is asynchronous. Do not check ctx here: the worker used to
			// defer cancel(), which canceled this context before this callback ran
			// and left the home page stuck in the loading state forever.
			defer cancel()
			if a.ctx.Err() != nil || a.home.request != request {
				return
			}
			a.home.recent, a.home.loadedAt, a.home.cancel, a.home.mods = recent, time.Now(), nil, mods
		})
	})
}

func worldModeLabel(world instancecontent.World) string {
	if world.Hardcore {
		return locales.Hardcore()
	}
	if world.Mode == 1 {
		return locales.Creative()
	}
	if world.Mode == 2 {
		return locales.Adventure()
	}
	if world.Mode == 3 {
		return locales.Spectator()
	}
	return locales.Survival()
}

func (a *app) cancelHomeScan() {
	if a.home.cancel != nil {
		a.home.cancel()
		a.home.cancel = nil
	}
	a.home.request++
	a.home.checkedAt = time.Time{}
}

func (a *app) homeRecent() []page.HomeRecent {
	recent := a.home.recentScratch[:0]
	recent = append(recent, a.home.recent...)
	for _, record := range a.playHistory(time.Now()).recentServer {
		if v, ok := a.instanceByKey(record.Instance); ok {
			recent = append(recent, page.HomeRecent{Instance: v, Name: record.Server, Detail: record.Server, Address: record.Server, Played: record.Started})
		}
	}
	slices.SortFunc(recent, func(a, b page.HomeRecent) int { return b.Played.Compare(a.Played) })
	recent = recent[:min(6, len(recent))]
	a.home.recentScratch = recent
	return recent
}

// Percent(0) needs the same Process across samples to report interval usage.
type processProbe struct{ procs map[int32]*process.Process }

func (g *processProbe) sample(ctx context.Context, pids []int, includeChildren bool) (float64, int) {
	if g.procs == nil {
		g.procs = make(map[int32]*process.Process)
	}
	seen := make(map[int32]bool)
	cpuTotal, rss := float64(0), 0
	var walk func(*process.Process)
	walk = func(p *process.Process) {
		if seen[p.Pid] {
			return
		}
		seen[p.Pid] = true
		cached, existed := g.procs[p.Pid]
		if existed {
			p = cached
		} else {
			g.procs[p.Pid] = p
		}
		if value, err := p.PercentWithContext(ctx, 0); err == nil && existed {
			cpuTotal += value
		}
		if mem, err := p.MemoryInfoWithContext(ctx); err == nil {
			rss += int(mem.RSS >> 20)
		}
		if includeChildren {
			if children, err := p.ChildrenWithContext(ctx); err == nil {
				for _, child := range children {
					walk(child)
				}
			}
		}
	}
	for _, pid := range pids {
		if p, err := process.NewProcessWithContext(ctx, int32(pid)); err == nil {
			walk(p)
		}
	}
	for pid := range g.procs {
		if !seen[pid] {
			delete(g.procs, pid)
		}
	}
	return cpuTotal / float64(runtime.NumCPU()), rss
}

func (a *app) homeStatistics(key string) page.HomeStatistics {
	now := time.Now()
	history := a.playHistory(now)
	s := page.HomeStatistics{CPU: a.home.cpu, CPUReady: a.home.cpuReady, LauncherMiB: a.home.launcherMiB, MemoryUsed: a.memory.UsedMiB, MemoryTotal: a.memory.TotalMiB}
	s.Days, s.Launches, s.LastPlayed = history.days[key], history.launches[key], history.lastPlayed[key]
	for instance, session := range a.games {
		if session == nil || session.playedAt.IsZero() {
			continue
		}
		played := history.sessions[playHistoryKey{instance: instance, started: session.playedAt.UnixNano()}]
		if played.count == 0 {
			continue
		}
		for i, value := range played.days {
			s.Days[i] -= value
		}
		addPlayDays(&s.Days, history.first, session.playedAt, int64(now.Sub(session.playedAt).Seconds()), float64(played.count))
	}
	for _, v := range a.installedList {
		if session := a.games[v.Key()]; session != nil && session.activity.Running {
			s.Running = v.Name
			break
		}
	}
	return s
}

func (a *app) recordPlayStarted(key string, session *gameSession) {
	session.playedAt = time.Now()
	a.cfg.PlayHistory = append(a.cfg.PlayHistory, config.PlayRecord{Instance: key, Started: session.playedAt, Server: session.server})
	if len(a.cfg.PlayHistory) > 4096 {
		a.cfg.PlayHistory = slices.Clone(a.cfg.PlayHistory[len(a.cfg.PlayHistory)-4096:])
	}
	a.invalidatePlayHistory()
	a.home.loadedAt = time.Time{}
	a.saveConfig()
}

func (a *app) recordPlayFinished(key string, session *gameSession) {
	if session.playedAt.IsZero() {
		return
	}
	for i := len(a.cfg.PlayHistory) - 1; i >= 0; i-- {
		r := &a.cfg.PlayHistory[i]
		if r.Instance == key && r.Started.Equal(session.playedAt) {
			r.Seconds = max(0, int64(time.Since(session.playedAt).Seconds()))
			break
		}
	}
	a.invalidatePlayHistory()
	a.home.loadedAt = time.Time{}
	a.saveConfig()
}

func (a *app) libraryStatistics() map[string]page.LibraryStatistics {
	now := time.Now()
	history := a.playHistory(now)
	stats := make(map[string]page.LibraryStatistics, len(a.installedList))
	for _, instance := range a.installedList {
		mods, known := a.home.mods[instance.Key()]
		if a.instanceContent.data.Key == instance.Key() && !a.instanceContent.data.Loading {
			mods, known = a.instanceContent.data.Summary.Resources[resource.Mod].Total, true
		}
		stats[instance.Key()] = page.LibraryStatistics{Mods: mods, Known: known}
	}
	for key, seconds := range history.totals {
		value, ok := stats[key]
		if !ok {
			continue
		}
		value.Seconds = seconds
		stats[key] = value
	}
	for key, session := range a.games {
		if session == nil || session.playedAt.IsZero() {
			continue
		}
		played := history.sessions[playHistoryKey{instance: key, started: session.playedAt.UnixNano()}]
		if played.count == 0 {
			continue
		}
		value, ok := stats[key]
		if !ok {
			continue
		}
		value.Seconds += max(0, int64(now.Sub(session.playedAt).Seconds()))*int64(played.count) - played.seconds
		stats[key] = value
	}
	return stats
}

func (a *app) flushPlayHistory() {
	for key, session := range a.games {
		if session.playedAt.IsZero() {
			continue
		}
		for i := len(a.cfg.PlayHistory) - 1; i >= 0; i-- {
			r := &a.cfg.PlayHistory[i]
			if r.Instance == key && r.Started.Equal(session.playedAt) {
				r.Seconds = max(0, int64(time.Since(session.playedAt).Seconds()))
				break
			}
		}
	}
}

func (a *app) openHomeRecent(recent page.HomeRecent) {
	a.goPage(pageInstances) // nav:link
	a.pages.instances.OpenRecent(recent.Instance.Key(), recent.Path, recent.Address != "")
}

func (a *app) launchHomeRecent(recent page.HomeRecent) {
	if recent.Address != "" {
		a.launchTarget(recent.Instance.Key(), launcher.ServerTarget{Address: recent.Address, Protocol: -1})
	} else {
		a.launchVersion(recent.Instance.Key())
	}
}
