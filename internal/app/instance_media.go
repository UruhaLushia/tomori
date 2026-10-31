package app

import (
	"context"
	"image"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/locales"
)

type contentImageStamp struct {
	size, modified int64
}

type contentThumbnail struct {
	stamp  contentImageStamp
	bitmap *ui.Bitmap
	used   uint64
	bytes  int
}

type contentWorld struct {
	stamp contentImageStamp
	world instancecontent.World
	used  uint64
}

func (a *app) releaseInstanceMedia() {
	a.pages.instances.ReleaseMedia()
	s := &a.instanceContent
	if s.mediaCancel != nil {
		s.mediaCancel()
	}
	s.mediaCtx, s.mediaCancel = nil, nil
	s.mediaGeneration++
	s.thumbnails, s.pending, s.data.Thumbnails = nil, nil, nil
	s.worlds, s.worldPending, s.data.Worlds = nil, nil, nil
	s.thumbnailBytes = 0
	a.closeScreenshotPreview()
}

func (a *app) contentMediaContext() context.Context {
	s := &a.instanceContent
	if s.mediaCtx == nil {
		s.mediaCtx, s.mediaCancel = context.WithCancel(a.ctx)
		s.thumbnails = make(map[string]contentThumbnail)
		s.pending = make(map[string]contentImageStamp)
		s.data.Thumbnails = make(map[string]*ui.Bitmap)
		s.worlds = make(map[string]contentWorld)
		s.worldPending = make(map[string]contentImageStamp)
		s.data.Worlds = make(map[string]instancecontent.World)
	}
	if s.mediaLane == nil {
		s.mediaLane = make(chan struct{}, 1)
	}
	return s.mediaCtx
}

func (a *app) loadWorldMetadata(file instancecontent.File) {
	ctx := a.contentMediaContext()
	s := &a.instanceContent
	stamp := contentImageStamp{file.Size, file.Modified.UnixNano()}
	s.thumbnailClock++
	if cached, ok := s.worlds[file.Path]; ok && cached.stamp == stamp {
		cached.used = s.thumbnailClock
		s.worlds[file.Path] = cached
		return
	}
	if pending, ok := s.worldPending[file.Path]; ok && pending == stamp || len(s.worldPending) >= 16 {
		return
	}
	if s.worldLane == nil {
		s.worldLane = make(chan struct{}, 1)
	}
	s.worldPending[file.Path] = stamp
	generation, lane := s.mediaGeneration, s.worldLane
	a.workers.Go(func() {
		select {
		case lane <- struct{}{}:
		case <-ctx.Done():
			return
		}
		world, err := instancecontent.ReadWorld(ctx, file.Path)
		<-lane
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			world = instancecontent.World{}
		}
		a.win.Update(func() {
			s := &a.instanceContent
			if a.ctx.Err() != nil || generation != s.mediaGeneration || s.worldPending[file.Path] != stamp {
				return
			}
			delete(s.worldPending, file.Path)
			if len(s.worlds) >= 128 {
				oldest, used := "", ^uint64(0)
				for path, cached := range s.worlds {
					if cached.used < used {
						oldest, used = path, cached.used
					}
				}
				delete(s.worlds, oldest)
				delete(s.data.Worlds, oldest)
			}
			s.thumbnailClock++
			s.worlds[file.Path] = contentWorld{stamp, world, s.thumbnailClock}
			s.data.Worlds[file.Path] = world
		})
	})
}

func (a *app) loadContentThumbnail(file instancecontent.File) {
	if file.Cover == "" {
		return
	}
	ctx := a.contentMediaContext()
	s := &a.instanceContent
	stamp := contentImageStamp{file.CoverSize, file.CoverModified.UnixNano()}
	s.thumbnailClock++
	if cached, ok := s.thumbnails[file.Cover]; ok && cached.stamp == stamp {
		cached.used = s.thumbnailClock
		s.thumbnails[file.Cover] = cached
		return
	}
	if pending, ok := s.pending[file.Cover]; ok && pending == stamp {
		return
	}
	if len(s.pending) >= 32 {
		return
	}
	s.pending[file.Cover] = stamp
	generation, lane, root := s.mediaGeneration, s.mediaLane, s.data.Root
	a.workers.Go(func() {
		select {
		case lane <- struct{}{}:
		case <-ctx.Done():
			return
		}
		var img image.Image
		if file.Cover == file.Path {
			img, _ = instancecontent.LoadScreenshot(ctx, root, file, 320, 180)
		} else {
			img, _ = instancecontent.LoadImage(ctx, file.Cover, 320, 180)
		}
		<-lane
		if ctx.Err() != nil {
			return
		}
		var bitmap *ui.Bitmap
		cost := 0
		if img != nil {
			bitmap = ui.NewBitmap(img)
			if file.Cover != file.Path {
				bitmap.Pixelated()
			}
			cost = img.Bounds().Dx() * img.Bounds().Dy() * 4
		}
		a.win.Update(func() {
			s := &a.instanceContent
			if a.ctx.Err() != nil || generation != s.mediaGeneration || s.pending[file.Cover] != stamp {
				return
			}
			delete(s.pending, file.Cover)
			s.storeThumbnail(file.Cover, stamp, bitmap, cost)
		})
	})
}

func (s *instanceContentState) storeThumbnail(path string, stamp contentImageStamp, bitmap *ui.Bitmap, cost int) {
	if old, ok := s.thumbnails[path]; ok {
		s.thumbnailBytes -= old.bytes
	}
	delete(s.thumbnails, path)
	for len(s.thumbnails) >= 48 || s.thumbnailBytes+cost > 8<<20 {
		oldest, used := "", ^uint64(0)
		for key, cached := range s.thumbnails {
			if cached.used < used {
				oldest, used = key, cached.used
			}
		}
		if oldest == "" {
			break
		}
		s.thumbnailBytes -= s.thumbnails[oldest].bytes
		delete(s.thumbnails, oldest)
		delete(s.data.Thumbnails, oldest)
	}
	s.thumbnailClock++
	s.thumbnails[path] = contentThumbnail{stamp, bitmap, s.thumbnailClock, cost}
	s.data.Thumbnails[path] = bitmap
	s.thumbnailBytes += cost
}

func (a *app) previewScreenshot(path string) {
	s := &a.instanceContent
	if s.data.PreviewPath == path && (s.data.PreviewLoading || s.data.Preview != nil) {
		return
	}
	var source instancecontent.File
	for _, file := range s.data.Summary.Screenshots {
		if file.Path == path {
			source = file
			break
		}
	}
	if source.Path == "" {
		return
	}
	a.closeScreenshotPreview()
	ctx, cancel := context.WithCancel(a.contentMediaContext())
	s.previewCancel = cancel
	s.data.PreviewPath, s.data.PreviewLoading = path, true
	token, lane, root := s.previewRequest, s.mediaLane, s.data.Root
	a.workers.Go(func() {
		defer cancel()
		select {
		case lane <- struct{}{}:
		case <-ctx.Done():
			return
		}
		img, err := instancecontent.LoadScreenshot(ctx, root, source, 1800, 1080)
		<-lane
		if ctx.Err() != nil {
			return
		}
		var bitmap *ui.Bitmap
		if img != nil {
			bitmap = ui.NewBitmap(img)
		}
		a.win.Update(func() {
			s := &a.instanceContent
			if a.ctx.Err() != nil || token != s.previewRequest || s.data.PreviewPath != path {
				return
			}
			s.data.Preview, s.data.PreviewLoading = bitmap, false
			if err != nil {
				s.data.PreviewError = locales.CannotPreviewThisImage()
			}
		})
	})
}

func (a *app) closeScreenshotPreview() {
	s := &a.instanceContent
	if s.previewCancel != nil {
		s.previewCancel()
		s.previewCancel = nil
	}
	s.previewRequest++
	s.data.PreviewPath, s.data.PreviewError = "", ""
	s.data.Preview, s.data.PreviewLoading = nil, false
}
