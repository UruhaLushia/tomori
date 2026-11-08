package page

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type PackBrowserData struct {
	ID, Revision            uint64
	Open, Loading, FullPage bool
	Error                   string
	Filter                  packcatalog.Filter
	Categories              []packcatalog.Category
	Games                   []string
	Result                  packcatalog.Result
	Project                 *packcatalog.Project
	Versions                []packcatalog.Version
	Icons                   map[string]*ui.Bitmap
	Animations              map[string][]byte
	Videos                  map[string]*ui.VideoPlayer
	InstallStatus           func(string) kit.InstallProgress
}
type PackBrowserActions struct {
	Close, Search, SourceChanged, Back func()
	Import                             func()
	Page                               func(int)
	Versions                           func(packcatalog.Project)
	Choose                             func(packcatalog.Version)
	EnsureVideo                        func(string)
	ReleaseVideos                      func(map[string]struct{})
}
type PackBrowser struct {
	list, versionList     ui.ListState
	revision              uint64
	project, game, loader string
}

func (p *PackBrowser) View(c *ui.Context, d *PackBrowserData, a PackBrowserActions) {
	if !d.Open {
		return
	}
	if p.revision != d.Revision {
		p.revision = d.Revision
		p.list = ui.ListState{}
	}
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		project := ""
		if d.Project != nil {
			project = d.Project.ID
			c.OnShortcut(0, ui.KeyEscape, a.Back)
		}
		kit.SlideMotion(ui.Column(c.Key("pack-content:"+project)).Fill().MinWidth(0).MinHeight(0).Gap(12), d.Project != nil).Children(func() { p.body(c, d, a) })
	})
}

func (p *PackBrowser) body(c *ui.Context, d *PackBrowserData, a PackBrowserActions) {
	if d.Project != nil {
		if p.project != d.Project.ID {
			p.project = d.Project.ID
			p.game = d.Filter.GameVersion
			p.loader = ""
			p.versionList = ui.ListState{}
		}
		p.detail(c, d, a)
		return
	}
	p.filters(c, d, a)
	count := len(d.Result.Projects)
	if count == 0 {
		switch {
		case d.Loading:
			kit.Loading(c)
		case d.Error != "":
			kit.EmptyState(c, kit.Download, locales.CouldNotLoad(), d.Error, func() { kit.Ghost(c, locales.Retry(), a.Search) })
		default:
			kit.EmptyState(c, kit.Cube, locales.NoMatchingModpacks(), locales.TryAnotherSearchOrAdjustTheFilters(), nil)
		}
		return
	}
	more := count < d.Result.Total
	c.Key("pack-results")
	visible := make(map[string]struct{})
	ui.List(c, &p.list, count+1).Grow(1).FillWidth().MinHeight(0).Margin(0, -8).ItemKey(func(i int) any {
		if i == count {
			return "pack-end"
		}
		return d.Result.Projects[i].ID
	}).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		if row.Index == count {
			kit.ListEnd(c, count, d.Loading, d.Error, more, func() { a.Page(d.Filter.Offset) })
			return
		}
		project := d.Result.Projects[row.Index]
		visible[project.ID] = struct{}{}
		kit.ListRow(c, kit.Row{Title: project.Name, Subtitle: project.Description, Tags: strings.Join(project.Categories, " · "), Lead: func() {
			if a.EnsureVideo != nil {
				a.EnsureVideo(project.ID)
			}
			kit.ProjectIcon(c, d.Icons[project.ID], 32, d.Videos[project.ID])
		}, OnClick: func() { a.Versions(project) }}, func(bool) {
			if !kit.Compact(c) {
				ui.Column(c).Width(kit.W(120)).Shrink(0).AlignItems(ui.End).Children(func() {
					kit.Line(c, downloadCount(project.Downloads)+" "+locales.Downloads6(), kit.Caption)
				})
			}
		})
	})
	if a.ReleaseVideos != nil {
		a.ReleaseVideos(visible)
	}
	if !d.Loading && d.Error == "" && more && kit.NearListEnd(c, &p.list, count) {
		a.Page(d.Result.Offset + packcatalog.PageSize)
	}
}

func (p *PackBrowser) filters(c *ui.Context, d *PackBrowserData, a PackBrowserActions) {
	categories := []kit.SelectOption[string]{{Value: "", Label: locales.All()}}
	category := locales.All()
	for _, v := range d.Categories {
		categories = append(categories, kit.SelectOption[string]{Value: v.ID, Label: v.Name})
		if v.ID == d.Filter.Category {
			category = v.Name
		}
	}
	games := []kit.SelectOption[string]{{Value: "", Label: locales.All()}}
	for _, v := range d.Games {
		games = append(games, kit.SelectOption[string]{Value: v, Label: v})
	}
	game := d.Filter.GameVersion
	if game == "" {
		game = locales.All()
	}
	kit.Toolbar(c, func() {
		kit.FilterSelect(c, &d.Filter.Source, locales.Source3()+" · "+map[string]string{packcatalog.Modrinth: "Modrinth", packcatalog.CurseForge: "CurseForge"}[d.Filter.Source], []kit.SelectOption[string]{{Value: packcatalog.Modrinth, Label: "Modrinth"}, {Value: packcatalog.CurseForge, Label: "CurseForge"}}, 180, a.SourceChanged)
		kit.FilterSelect(c, &d.Filter.Category, locales.Category()+" · "+category, categories, 240, a.Search)
		kit.FilterSelect(c, &d.Filter.GameVersion, locales.GameVersion()+" · "+game, games, 200, a.Search)
		kit.FilterSelect(c, &d.Filter.Sort, locales.Sort()+" · "+map[string]string{"popular": locales.Popularity(), "downloads": locales.Downloads5(), "updated": locales.RecentlyUpdated(), "newest": locales.Newest()}[d.Filter.Sort], []kit.SelectOption[string]{{Value: "popular", Label: locales.Popularity()}, {Value: "downloads", Label: locales.Downloads5()}, {Value: "updated", Label: locales.RecentlyUpdated()}, {Value: "newest", Label: locales.Newest()}}, 200, a.Search)
		ui.Spacer(c)
		kit.SearchField(c, &d.Filter.Query, locales.SearchModpacks(), d.Loading).OnSubmit(a.Search)
	})
}

func (p *PackBrowser) detail(c *ui.Context, d *PackBrowserData, a PackBrowserActions) {
	services := c.Services()
	ui.Column(c).FillWidth().Shrink(0).Gap(12).Children(func() {
		kit.BackBar(c, locales.Modpack(), a.Back, nil)
		kit.IdentityHeader(c, kit.Identity{Icon: d.Icons[d.Project.ID], Video: d.Videos[d.Project.ID], Name: d.Project.Name, Meta: kit.MetaLine(d.Project.Author, downloadCount(d.Project.Downloads)+" "+locales.Downloads6())}, nil, func() {
			if d.Project.Website != "" {
				kit.IconButton(c, kit.External, false, func() { services.OpenURL(d.Project.Website) }).Label(locales.OpenModpackWebsite())
			}
		})
	})
	ui.Box(c).Height(8).Shrink(0)
	games := []kit.SelectOption[string]{{Value: "", Label: locales.All()}}
	loaders := []kit.SelectOption[string]{{Value: "", Label: locales.All()}}
	for _, v := range d.Versions {
		for _, g := range v.GameVersions {
			if !slices.ContainsFunc(games, func(o kit.SelectOption[string]) bool { return o.Value == g }) {
				games = append(games, kit.SelectOption[string]{Value: g, Label: g})
			}
		}
		for _, l := range v.Loaders {
			if !slices.ContainsFunc(loaders, func(o kit.SelectOption[string]) bool { return o.Value == l }) {
				loaders = append(loaders, kit.SelectOption[string]{Value: l, Label: loaderLabel(l)})
			}
		}
	}
	game, loader := p.game, p.loader
	if game == "" {
		game = locales.All()
	}
	if loader == "" {
		loader = locales.All()
	}
	kit.Toolbar(c, func() {
		kit.FilterSelect(c, &p.game, locales.GameVersion()+" · "+game, games, 200, func() { p.versionList = ui.ListState{} })
		kit.FilterSelect(c, &p.loader, locales.Loader()+" · "+loader, loaders, 200, func() { p.versionList = ui.ListState{} })
	})
	var versions []packcatalog.Version
	for _, v := range d.Versions {
		if p.game != "" && !slices.Contains(v.GameVersions, p.game) {
			continue
		}
		if p.loader != "" && !slices.Contains(v.Loaders, p.loader) {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		if d.Loading {
			kit.Loading(c)
		} else {
			kit.EmptyState(c, kit.Cube, locales.NoMatchingVersions(), d.Error, nil)
		}
		return
	}
	c.Key("pack-versions")
	ui.List(c, &p.versionList, len(versions)+1).Grow(1).FillWidth().MinHeight(0).Margin(0, -8).ItemKey(func(i int) any {
		if i == len(versions) {
			return "pack-versions-end"
		}
		return versions[i].ID
	}).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		if row.Index == len(versions) {
			kit.ListEnd(c, len(versions), false, "", false, nil)
			return
		}
		v := versions[row.Index]
		label := locales.Install()
		if v.URL == "" {
			label = locales.DownloadInBrowser()
		}
		title := v.Filename
		if title == "" {
			title = v.Number
		}
		if title == "" {
			title = v.Name
		}
		progress := kit.InstallProgress{}
		if d.InstallStatus != nil {
			progress = d.InstallStatus(v.OperationKey())
		}
		kit.VersionRow(c, kit.VersionRowData{Title: title, Subtitle: v.Name, Channel: locales.Release(), Tone: kit.SuccessTone, Compatibility: kit.MetaLine(strings.Join(v.GameVersions, " / "), strings.Join(v.Loaders, " / ")), Detail: kit.MetaLine(v.Published.Format("2006-01-02"), fmt.Sprintf("%.1f MiB", float64(v.Size)/(1<<20))), Action: label, Installing: progress, Disabled: progress.Active || v.URL == "" && d.Project.Website == ""}, func() {
			if progress.Active {
				return
			}
			if v.URL == "" {
				services.OpenURL(strings.TrimRight(d.Project.Website, "/") + "/files/" + v.ID)
				return
			}
			a.Choose(v)
		})
	})
}
