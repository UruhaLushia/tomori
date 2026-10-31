package app

import (
	"context"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
)

type instanceContentState struct {
	data            page.InstanceContentData
	request         uint64
	cancel          context.CancelFunc
	mediaCtx        context.Context
	mediaCancel     context.CancelFunc
	mediaGeneration uint64
	mediaLane       chan struct{}
	thumbnails      map[string]contentThumbnail
	pending         map[string]contentImageStamp
	thumbnailClock  uint64
	thumbnailBytes  int
	worlds          map[string]contentWorld
	worldPending    map[string]contentImageStamp
	worldLane       chan struct{}
	previewRequest  uint64
	previewCancel   context.CancelFunc
}

func (a *app) resetInstanceContent() {
	a.releaseInstanceMedia()
	s := &a.instanceContent
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.request++
	s.data = page.InstanceContentData{}
}

func (a *app) inspectInstance(instance minecraft.Instance, refresh bool) {
	s := &a.instanceContent
	root := instance.ContentDirectory(a.cfg.GameForInstance(instance.Key()).Isolate)
	if !refresh && s.data.Key == instance.Key() && s.data.Root == root {
		return
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.request++
	token := s.request
	ctx, cancel := context.WithCancel(a.ctx)
	s.cancel = cancel
	if s.data.Key != instance.Key() || s.data.Root != root {
		a.releaseInstanceMedia()
		s.data = page.InstanceContentData{Key: instance.Key(), Root: root}
		a.closeResources()
	}
	s.data.Loading, s.data.Error = true, ""
	a.workers.Go(func() {
		defer cancel()
		summary, err := instancecontent.Scan(ctx, root)
		if ctx.Err() != nil {
			return
		}
		a.win.Update(func() {
			if token != a.instanceContent.request || a.ctx.Err() != nil {
				return
			}
			s := &a.instanceContent
			s.data.Summary, s.data.Loading = summary, false
			if err != nil {
				s.data.Error = err.Error()
			}
		})
	})
}
