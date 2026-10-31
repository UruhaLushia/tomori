package app

import (
	"context"
	"os"
	"time"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/locales"
)

type cacheUsageState struct {
	directory string
	bytes     uint64
	known     bool
	updated   time.Time
	id        uint64
	cancel    context.CancelFunc
	err       error
}

const cacheUsageInterval = 5 * time.Second

func (a *app) refreshCacheUsage(force bool) {
	if a.win == nil || a.ctx.Err() != nil {
		return
	}
	s, dir := &a.cacheUsage, a.cfg.CacheDirectory()
	if !force && s.directory == dir && (s.cancel != nil || time.Since(s.updated) < cacheUsageInterval) {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.directory != dir {
		s.bytes, s.known = 0, false
	}
	s.directory, s.err = dir, nil
	s.id++
	id := s.id
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	a.workers.Go(func() {
		defer cancel()
		size, err := download.CacheSize(ctx, dir)
		a.win.Update(func() {
			if a.ctx.Err() != nil || s.id != id || a.cfg.CacheDirectory() != dir {
				return
			}
			s.cancel, s.updated, s.err = nil, time.Now(), err
			s.known = err == nil
			if err == nil {
				s.bytes = size
			}
		})
	})
}

func (a *app) prepareCache() {
	a.pathBusy = true
	dir := a.cfg.CacheDirectory()
	a.workers.Go(func() {
		err := os.MkdirAll(dir, 0o755)
		if err == nil {
			err = download.PruneCache(a.ctx, dir)
		}
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.pathBusy = false
			a.refreshCacheUsage(true)
			if err != nil {
				a.cacheError = err.Error()
				a.logf(locales.FailedToPruneCache(), err)
			}
			a.loadManifest()
		})
	})
}

func (a *app) changeCacheDirectory() {
	if a.hasTasks() || a.pathBusy || a.manifestLoading {
		return
	}
	a.openDirectoryPicker(pickerCache, a.cfg.CacheDirectory())
}

func (a *app) clearCache() {
	if a.hasTasks() || a.pathBusy || a.manifestLoading {
		return
	}
	a.pathBusy, a.cacheError, a.cacheStatus = true, "", locales.Clearing()
	dir := a.cfg.CacheDirectory()
	a.workers.Go(func() {
		err := download.ClearCache(dir)
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.pathBusy, a.cacheStatus = false, ""
			a.refreshCacheUsage(true)
			if err != nil {
				a.cacheError = err.Error()
				a.logf(locales.FailedToClearCache(), err)
				a.notifyError(locales.CouldNotClearCache(), err)
			} else {
				a.cacheStatus = locales.Cleared()
				a.logf("%s", locales.CacheCleared())
				a.notify(locales.CacheCleared(), "", false)
			}
		})
	})
}
