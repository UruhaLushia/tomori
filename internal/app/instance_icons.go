package app

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type cachedInstanceIcon struct {
	signature  [32]byte
	bitmap     *ui.Bitmap
	generation uint64
	used       uint64
}

type instanceIconState struct {
	ctx        context.Context
	cancel     context.CancelFunc
	generation uint64
	clock      uint64
	lane       chan struct{}
	pending    map[string]bool
	cache      map[string]cachedInstanceIcon
	bitmaps    map[string]*ui.Bitmap
}

func (s *instanceIconState) invalidate() {
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel, s.pending = nil, nil, nil
	s.generation++
}

func (s *instanceIconState) release() {
	s.invalidate()
	s.cache, s.bitmaps = nil, nil
}

func (s *instanceIconState) store(key string, icon cachedInstanceIcon) {
	delete(s.cache, key)
	for len(s.cache) >= 64 {
		oldest, used := "", ^uint64(0)
		for candidate, entry := range s.cache {
			if entry.used < used {
				oldest, used = candidate, entry.used
			}
		}
		delete(s.cache, oldest)
		delete(s.bitmaps, oldest)
	}
	s.clock++
	icon.used, icon.generation = s.clock, s.generation
	s.cache[key], s.bitmaps[key] = icon, icon.bitmap
}

func (s *instanceIconState) retain(installed map[string]bool) {
	for key := range s.cache {
		if _, ok := installed[key]; !ok {
			delete(s.cache, key)
			delete(s.bitmaps, key)
		}
	}
}

func (s *instanceIconState) complete(generation uint64, key string, icon cachedInstanceIcon) bool {
	if generation != s.generation || !s.pending[key] {
		return false
	}
	delete(s.pending, key)
	s.store(key, icon)
	return true
}

func (a *app) loadInstanceIcon(instance minecraft.Instance) {
	if !a.instancesReady || a.ctx.Err() != nil {
		return
	}
	s := &a.icons
	key := instance.Key()
	previous, ok := s.cache[key]
	if ok && previous.generation == s.generation {
		s.clock++
		previous.used = s.clock
		s.cache[key] = previous
		return
	}
	if s.pending[key] || len(s.pending) >= 16 {
		return
	}
	a.queueInstanceIcon(key, instance, previous)
}

func (a *app) queueInstanceIcon(key string, instance minecraft.Instance, previous cachedInstanceIcon) {
	s := &a.icons
	if s.ctx == nil {
		s.ctx, s.cancel = context.WithCancel(a.ctx)
		s.pending = make(map[string]bool)
		if s.cache == nil {
			s.cache = make(map[string]cachedInstanceIcon)
			s.bitmaps = make(map[string]*ui.Bitmap)
		}
		if s.lane == nil {
			s.lane = make(chan struct{}, 1)
		}
	}
	s.pending[key] = true
	ctx, generation, lane, budget := s.ctx, s.generation, s.lane, a.downloadBudget
	a.workers.Go(func() {
		select {
		case lane <- struct{}{}:
		case <-ctx.Done():
			return
		}
		icon, err := readInstanceIcon(ctx, budget, instance, previous)
		<-lane
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			s := &a.icons
			if a.ctx.Err() != nil || !s.complete(generation, key, icon) {
				return
			}
			if a.instanceSettings.Open && a.instanceSettings.Key == key {
				a.instanceSettings.Icon = icon.bitmap
			}
			if a.instanceEdit.data.Open && a.instanceEdit.data.Key == key && !a.instanceEdit.data.Changed {
				a.instanceEdit.data.Icon = icon.bitmap
			}
			if err != nil {
				a.logf(locales.InstanceScan(), err)
			}
		})
	})
}

func readInstanceIcon(ctx context.Context, budget *download.Budget, instance minecraft.Instance, previous cachedInstanceIcon) (cachedInstanceIcon, error) {
	release, err := budget.FilePermit(ctx)
	if err != nil {
		return cachedInstanceIcon{}, err
	}
	defer release()
	signature, err := minecraft.InstanceIconSignature(ctx, instance)
	if err != nil {
		return cachedInstanceIcon{}, err
	}
	if previous.signature == signature {
		return previous, nil
	}
	icon, err := minecraft.LoadInstanceIcon(ctx, instance)
	entry := cachedInstanceIcon{signature: signature}
	if icon != nil {
		entry.bitmap = ui.NewBitmap(icon)
	}
	return entry, err
}
