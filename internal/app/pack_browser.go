package app

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
)

type packBrowserState struct {
	data    page.PackBrowserData
	request uint64
	cancel  context.CancelFunc
}

func (a *app) ensurePackVideo(id string) {
	p := &a.packBrowser.data
	if len(p.Animations[id]) == 0 {
		return
	}
	if p.Videos == nil {
		p.Videos = make(map[string]*ui.VideoPlayer)
	}
	if p.Videos[id] != nil {
		return
	}
	video, err := kit.AnimatedVideo(p.Animations[id])
	if err == nil {
		p.Videos[id] = video
	}
}

func (a *app) releasePackVideos(keep map[string]struct{}) {
	p := &a.packBrowser.data
	for id, video := range p.Videos {
		if _, ok := keep[id]; ok {
			continue
		}
		video.Close()
		delete(p.Videos, id)
	}
}

func (a *app) cancelPackBrowser() {
	p := &a.packBrowser
	p.request++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}
func (a *app) openPackBrowser() {
	a.downloadCategory = downloadCategoryPack
	a.goPage(pageDownload) // nav:link
	a.openDownloadPackBrowser()
}

func (a *app) preparePackBrowser() {
	if a.tasksFull() || a.pathBusy {
		return
	}
	p := &a.packBrowser
	if p.data.Filter.Source == "" {
		p.data = page.PackBrowserData{Filter: packcatalog.Filter{Source: packcatalog.CurseForge, Sort: "popular"}}
	}
	p.data.Open = true
	p.data.Games = nil
	for _, v := range a.versions {
		if v.Type == "release" {
			p.data.Games = append(p.data.Games, v.ID)
		}
	}
	if len(p.data.Result.Projects) == 0 && p.data.Project == nil {
		a.searchPacks(0)
	}
}

func (a *app) openDownloadPackBrowser() {
	a.preparePackBrowser()
	a.packBrowser.data.FullPage = true
}

func (a *app) closeDownloadPackBrowser() {
	a.cancelPackBrowser()
	a.packBrowser.data.Open = false
	a.packBrowser.data.FullPage = false
	a.packBrowser.data.Loading = false
	a.releasePackBrowserResults()
}

func (a *app) packBrowserActions(fullPage bool) page.PackBrowserActions {
	close := a.backFromPackBrowser
	if fullPage {
		close = a.closeDownloadPackBrowser
	}
	return page.PackBrowserActions{
		Close:         close,
		Search:        func() { a.searchPacks(0) },
		SourceChanged: a.packSourceChanged,
		Import:        a.openPackImport,
		Page:          a.searchPacks,
		Versions:      a.packVersions,
		EnsureVideo:   a.ensurePackVideo,
		ReleaseVideos: a.releasePackVideos,
		Back: func() {
			a.cancelPackBrowser()
			for _, video := range a.packBrowser.data.Videos {
				video.Close()
			}
			a.packBrowser.data.Videos = make(map[string]*ui.VideoPlayer)
			a.packBrowser.data.Project = nil
			a.packBrowser.data.Loading = false
			a.packBrowser.data.Error = ""
		},
		Choose: func(v packcatalog.Version) { a.inspectPackInput("", &v, "online", "") },
	}
}

func (a *app) downloadPackBrowserActions() page.PackBrowserActions {
	return a.packBrowserActions(true)
}
func (a *app) backFromPackBrowser() {
	a.cancelPackBrowser()
	a.packBrowser.data.Open = false
	a.packBrowser.data.FullPage = false
	a.packBrowser.data.Loading = false
	a.releasePackBrowserResults()
	a.dialogs.pack.data.Open = true
}

func (a *app) releasePackBrowserResults() {
	p := &a.packBrowser.data
	for _, video := range p.Videos {
		video.Close()
	}
	p.Result = packcatalog.Result{}
	p.Project, p.Versions, p.Icons, p.Animations, p.Videos, p.Games = nil, nil, nil, nil, nil, nil
}
func (a *app) packSourceChanged() {
	p := &a.packBrowser.data
	p.Filter.Category = ""
	p.Categories = nil
	a.searchPacks(0)
}
func (a *app) packBrowserContext() (context.Context, context.CancelFunc, uint64) {
	a.cancelPackBrowser()
	p := &a.packBrowser
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	p.cancel = cancel
	p.data.ID = p.request
	p.data.Loading, p.data.Error = true, ""
	return ctx, cancel, p.request
}
func (a *app) searchPacks(offset int) {
	p := &a.packBrowser
	if !p.data.Open {
		return
	}
	ctx, cancel, token := a.packBrowserContext()
	p.data.Filter.Offset = offset
	p.data.Project = nil
	previousProjects := slices.Clone(p.data.Result.Projects)
	filter := p.data.Filter
	needCategories := len(p.data.Categories) == 0
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		result, err := service.PackSearch(ctx, filter)
		var categories []packcatalog.Category
		if err == nil && needCategories {
			categories, _ = service.PackCategories(ctx, filter.Source)
		}
		a.win.Update(func() {
			if token != p.request || a.ctx.Err() != nil || !p.data.Open {
				return
			}
			p.data.Loading = false
			if err != nil {
				p.data.Error = err.Error()
				return
			}
			if offset > 0 {
				previous := len(p.data.Result.Projects)
				seen := make(map[string]bool, len(p.data.Result.Projects))
				for _, project := range p.data.Result.Projects {
					seen[project.ID] = true
				}
				for _, project := range result.Projects {
					if !seen[project.ID] {
						p.data.Result.Projects = append(p.data.Result.Projects, project)
						seen[project.ID] = true
					}
				}
				p.data.Result.Total, p.data.Result.Offset = result.Total, offset
				if len(p.data.Result.Projects) == previous || offset+len(result.Projects) >= result.Total {
					p.data.Result.Total = len(p.data.Result.Projects)
				}
			} else {
				for _, video := range p.data.Videos {
					video.Close()
				}
				p.data.Result = result
				p.data.Revision++
				p.data.Icons = make(map[string]*ui.Bitmap)
				p.data.Animations = make(map[string][]byte)
				p.data.Videos = make(map[string]*ui.VideoPlayer)
			}
			if categories != nil {
				p.data.Categories = categories
			}
		})
		if err != nil {
			return
		}
		// Bound cover work separately from the download connection limit.
		projects := result.Projects
		if offset > 0 {
			seen := make(map[string]bool, len(previousProjects)+len(result.Projects))
			projects = make([]packcatalog.Project, 0, len(previousProjects)+len(result.Projects))
			for _, project := range append(previousProjects, result.Projects...) {
				if seen[project.ID] {
					continue
				}
				seen[project.ID] = true
				projects = append(projects, project)
			}
		}
		jobs := make(chan packcatalog.Project, len(projects))
		for _, project := range projects {
			if project.IconURL != "" {
				jobs <- project
			}
		}
		close(jobs)
		var group sync.WaitGroup
		for range 4 {
			group.Go(func() {
				for project := range jobs {
					if ctx.Err() != nil {
						return
					}
					cover, err := service.PackCover(ctx, project.IconURL)
					if err != nil {
						continue
					}
					bitmap := ui.NewBitmap(cover.Image)
					a.win.Update(func() {
						if token == p.request && p.data.Open && a.ctx.Err() == nil {
							p.data.Icons[project.ID] = bitmap
							if len(cover.Animation) > 0 {
								storeAnimation(p.data.Animations, project.ID, cover.Animation)
							}
						}
					})
				}
			})
		}
		group.Wait()
	})
}
func (a *app) packVersions(project packcatalog.Project) {
	p := &a.packBrowser
	if !p.data.Open {
		return
	}
	a.ensurePackVideo(project.ID)
	ctx, cancel, token := a.packBrowserContext()
	p.data.Project = &project
	p.data.Versions = nil
	source, game := p.data.Filter.Source, ""
	service := a.newService()
	a.workers.Go(func() {
		defer cancel()
		defer service.Close()
		if project.IconURL != "" {
			if cover, coverErr := service.PackCover(ctx, project.IconURL); coverErr == nil {
				bitmap := ui.NewBitmap(cover.Image)
				a.win.Update(func() {
					if token != p.request || !p.data.Open || a.ctx.Err() != nil {
						return
					}
					if p.data.Icons == nil {
						p.data.Icons = make(map[string]*ui.Bitmap)
					}
					p.data.Icons[project.ID] = bitmap
					if len(cover.Animation) > 0 {
						if p.data.Animations == nil {
							p.data.Animations = make(map[string][]byte)
						}
						storeAnimation(p.data.Animations, project.ID, cover.Animation)
						if p.data.Videos == nil {
							p.data.Videos = make(map[string]*ui.VideoPlayer)
						}
						a.ensurePackVideo(project.ID)
					}
				})
			}
		}
		versions, err := service.PackVersions(ctx, source, project.ID, game)
		a.win.Update(func() {
			if token != p.request || a.ctx.Err() != nil || !p.data.Open {
				return
			}
			p.data.Loading = false
			if err != nil {
				p.data.Error = err.Error()
				return
			}
			p.data.Versions = versions
		})
	})
}
