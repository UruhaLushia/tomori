package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type InstancesData struct {
	Statistics         map[string]LibraryStatistics
	ViewMode           *int
	Space              SpaceData
	Loaders            LoaderCatalogData
	Installed          []minecraft.Instance
	Versions           []minecraft.VersionSummary
	ManifestLoading    bool
	ManifestError      string
	InstallDirectories []config.GameDirectory
	Busy               bool
	InstanceBusy       bool
	Activity           func(string) InstanceActivity
	InUse              func(string) bool
	ContentRoot        func(minecraft.Instance) string
	ActiveInstance     string
	Status             string
	Icons              map[string]*ui.Bitmap
	Settings           map[string]config.InstanceSettings
	Content            InstanceContentData
	Resources          *ResourcesData
	Servers            *ServersData
}

type InstanceActivity struct {
	Active, Running, Canceling, Finishing bool
	Status                                string
}

type InstancesActions struct {
	New             func()
	ViewChanged     func()
	AssessInstall   func(string, string, modpack.Loader) SpaceData
	Reorder         func(key, before string)
	Install         func(string, string, modpack.Loader, string)
	Loaders         func(string, string)
	Repair          func(string)
	Launch          func(string)
	Stop            func(string)
	Refresh         func()
	Open            func(string)
	OpenFile        func(string)
	Settings        func(string)
	Edit            func(string, string)
	Components      func(string)
	Copy            func(string)
	Shortcut        func(string)
	CopyLink        func(string)
	Export          func(string)
	UpdatePack      func(string)
	AssociatePack   func(string)
	RestorePack     func(string)
	Import          func()
	BrowsePacks     func()
	RefreshContent  func()
	Content         func(resource.Kind)
	ResourceActions ResourcesActions
	LoadThumbnail   func(instancecontent.File)
	LoadIcon        func(minecraft.Instance)
	LoadWorld       func(instancecontent.File)
	EditWorld       func(instancecontent.File, string)
	WorldArchive    func(string, instancecontent.File)
	WorldPacks      func(instancecontent.File)
	Preview         func(string)
	ClosePreview    func()
	EditScreenshot  func(instancecontent.File, string)
	Leave           func()
	ServerActions   ServersActions
}

type Instances struct {
	selected           string
	installAction      string
	grid               ui.GridState
	libraryRows        ui.ListState
	reorder            instanceReorder
	tab                string
	tabIndex           int
	tabForward         bool
	tabScroll          ui.ScrollState
	tabReveal          bool
	tabHandles         [len(instanceTabs)]ui.Handle
	tabDrag            instanceTabDrag
	files              ui.ListState
	serverList         ui.ListState
	gallery            ui.GridState
	worldPath          string
	detail             bool
	inline             bool
	viewMode           int
	resources          Resources
	query, filterQuery string
	filterSource       *minecraft.Instance
	filterCount        int
	filtered           []minecraft.Instance
	revealKey          string
	overviewGrid       ui.Handle
}

func (p *Instances) Selected(installed []minecraft.Instance) (minecraft.Instance, bool) {
	for _, v := range installed {
		if v.Key() == p.selected {
			return v, true
		}
	}
	if len(installed) == 0 {
		p.detail = false
		return minecraft.Instance{}, false
	}
	p.selectInstance(installed[0].Key())
	return installed[0], true
}

func (p *Instances) DetailOpen() bool { return p.detail }

func (p *Instances) SetViewMode(mode int, hasInstances bool) {
	if p.viewMode != mode {
		p.reorder.cancel()
		p.grid = ui.GridState{}
		p.viewMode = mode
	}
	p.inline = mode == 0
	if p.inline && hasInstances {
		p.detail = true
	}
}

func (p *Instances) CanGoBack() bool { return p.detail && !p.inline }

func (p *Instances) Reveal(key string) { p.revealKey = key }

func (p *Instances) CloseDetail() {
	p.detail = false
	p.worldPath = ""
	p.tabDrag = instanceTabDrag{}
}

func (p *Instances) CancelInteractions() {
	p.reorder.cancel()
	p.tabDrag = instanceTabDrag{}
}

func (p *Instances) leaveDetail(actions InstancesActions) {
	p.CloseDetail()
	if actions.Leave != nil {
		actions.Leave()
	}
}

func (p *Instances) Open(key string) {
	p.reorder.cancel()
	p.detail = true
	p.selectInstance(key)
}

func (p *Instances) selectInstance(key string) {
	if p.selected != key {
		p.reorder.cancel()
		p.worldPath = ""
		p.selected, p.tab, p.tabIndex = key, "", 0
		p.tabScroll, p.tabReveal = ui.ScrollState{}, true
		p.tabDrag = instanceTabDrag{}
		p.files = ui.ListState{}
		p.gallery = ui.GridState{}
	}
}

func (p *Instances) OpenRecent(key, path string, server bool) {
	p.Open(key)
	p.tab, p.tabIndex = "worlds", 1
	if server {
		p.tab, p.tabIndex = "servers", 6
	}
	p.tabReveal = true
	p.worldPath = path
}

func (p *Instances) ServersOpen() bool { return p.detail && p.tab == "servers" }

func (p *Instances) ResourceKind() resource.Kind {
	if !p.detail {
		return ""
	}
	return resource.Kind(p.tab)
}

func (p *Instances) ReleaseResources() { p.resources = Resources{} }

func (p *Instances) ReleaseMedia() { p.gallery, p.worldPath = ui.GridState{}, "" }

func (p *Instances) View(c *ui.Context, data InstancesData, actions InstancesActions) {
	pad := kit.Pad(c)
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Padding(pad, pad, 0, pad).Gap(kit.Gap(c)).Children(func() {
		p.view(c, data, actions)
	})
}

func (p *Instances) view(c *ui.Context, data InstancesData, actions InstancesActions) {
	installed := data.Installed
	if len(installed) == 0 {
		p.detail = false
	}
	if p.detail && !p.inline {
		c.Key("instance-detail")
		c.OnShortcut(0, ui.KeyEscape, func() { p.leaveDetail(actions) })
		kit.SlideMotion(ui.Column(c).Fill().MinWidth(0).MinHeight(0), true).Children(func() {
			p.instanceDetail(c, installed, data, actions)
		})
	} else {
		c.Key("instance-list:" + fmt.Sprint(p.viewMode))
		kit.SlideMotion(ui.Column(c).Fill().MinWidth(0).MinHeight(0).Gap(kit.Gap(c)), false).Children(func() {
			if p.inline {
				if len(installed) == 0 {
					kit.EmptyState(c, kit.Cube, locales.NoGameInstances(), locales.CreateAnInstanceOrImportAModpackToGetStarted(), func() {
						kit.Primary(c, locales.NewInstance(), actions.New)
					})
					return
				}
				visible := p.filterInstances(installed)
				p.inlineLibrary(c, visible, installed, data, actions)
				return
			}
			kit.PageHeader(c, locales.Library(), fmt.Sprintf(locales.Instances(), len(installed)), nil, func() {
				searchWidth := float32(220)
				if kit.Compact(c) {
					searchWidth = 160
				}
				kit.SearchInput(c, &p.query, locales.SearchInstances()).Width(searchWidth)
				p.viewSwitcher(c, data, actions)
				kit.IconButton(c, kit.Refresh, false, actions.Refresh).Label(locales.RefreshInstances())
				p.installMenu(c, data.Busy, actions.New, actions.Import, actions.BrowsePacks)
			})
			if len(installed) == 0 {
				kit.EmptyState(c, kit.Cube, locales.NoGameInstances(), locales.CreateAnInstanceOrImportAModpackToGetStarted(), func() {
					kit.Primary(c, locales.NewInstance(), actions.New)
				})
				return
			}
			visible := p.filterInstances(installed)
			if len(visible) == 0 {
				kit.EmptyState(c, nil, locales.NoMatchingInstances(), "", nil)
				return
			}
			p.instanceList(c, visible, data, actions)
		})
	}
	p.screenshotPreview(c, data, actions)
	p.worldPreview(c, data, actions)
}

type instanceCardMotionKey struct {
	page       *Instances
	key        string
	generation uint64
}

func (p *Instances) inlineLibrary(c *ui.Context, visible, installed []minecraft.Instance, data InstancesData, actions InstancesActions) {
	previous := p.selected
	if len(visible) > 0 {
		p.Selected(visible)
	}
	if p.selected != previous {
		c.After(0)
	}
	listWidth := float32(248)
	if kit.Compact(c) {
		listWidth = 200
	}
	ui.Row(c).Grow(1).FillWidth().MinHeight(0).Gap(kit.ColGap(c)).AlignItems(ui.Stretch).Children(func() {
		ui.Column(c).Width(listWidth).Shrink(0).FillHeight().MinHeight(0).Gap(kit.S12).Children(func() {
			ui.Row(c).FillWidth().Height(kit.ControlH()).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
				kit.SearchInput(c, &p.query, locales.SearchInstances()).Grow(1).MinWidth(0)
				kit.IconButton(c, kit.Grid, false, func() {
					p.reorder.cancel()
					p.grid = ui.GridState{}
					p.viewMode, p.inline, p.detail = 1, false, false
					if data.ViewMode != nil {
						*data.ViewMode = p.viewMode
					}
					if actions.ViewChanged != nil {
						actions.ViewChanged()
					}
				}).Border(1, kit.LineColor).Label(locales.GridView())
				p.installMenu(c, data.Busy, actions.New, actions.Import, actions.BrowsePacks)
			})
			ui.Row(c).FillWidth().Height(kit.H(24, kit.Caption)).AlignItems(ui.Center).Shrink(0).Children(func() {
				kit.Line(c, locales.AllInstances(), kit.Strong).Grow(1)
				kit.Line(c, fmt.Sprintf("%d", len(installed)), kit.Caption).TextColor(kit.Muted)
			})
			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
				p.instanceList(c, visible, data, actions)
			})
		})
		ui.Box(c).Width(1).Shrink(0).FillHeight().Background(kit.LineSoft)
		if len(visible) > 0 {
			p.instanceDetail(c, visible, data, actions)
		} else {
			kit.EmptyState(c, nil, locales.NoMatchingInstances(), "", nil).Grow(1)
		}
	})
}

func (p *Instances) instanceList(c *ui.Context, installed []minecraft.Instance, data InstancesData, actions InstancesActions) {
	cols := 1
	if !p.inline {
		width := p.grid.Bounds(c).W
		if width <= 0 {
			c.After(0)
		}
		cols = max(1, int((width+8)/(kit.W(kit.CellMinWidth)+8)))
	}
	rows := (len(installed) + cols - 1) / cols
	p.prepareInstanceReorder(c, installed, actions.Reorder != nil)
	if p.revealKey != "" {
		for index, instance := range installed {
			if instance.Key() == p.revealKey {
				p.libraryRows.ScrollTo(index/cols, ui.Center)
				p.revealKey = ""
				break
			}
		}
	}
	preview := p.reorder.preview
	geometry := p.reorder.geometry(c, kit.CellH())
	oldSlots := append([]instanceSlot(nil), p.reorder.slots...)
	p.reorder.slots = p.reorder.slots[:0]
	p.reorder.passthrough = p.reorder.passthrough[:0]
	var insertionTarget ui.Element
	c.Key("instance-grid")
	grid := ui.List(c, &p.libraryRows, rows).Grow(1).FillWidth().MinHeight(0).Bind(&p.grid.Handle).ItemKey(func(i int) any {
		return installed[i*cols].Key()
	}).Scrollbars(false).Rows(func(item ui.ListRow) {
		c := item.Context
		start, end := item.Index*cols, min((item.Index+1)*cols, len(installed))
		gap, height, bottom := float32(8), kit.CellH(), float32(8)
		if item.Index+1 == rows {
			bottom = 0
		}
		if p.inline {
			gap, height, bottom = 0, kit.RowDoubleH(), 0
		}
		ui.Row(c).FillWidth().Height(height).Margin(0, 0, bottom, 0).Gap(gap).Children(func() {
			for i := start; i < end; i++ {
				v := installed[i]
				if actions.LoadIcon != nil {
					actions.LoadIcon(v)
				}
				c.Key(v.Key())
				ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(height).Children(func() {
					var slot ui.Element
					if p.inline {
						row := kit.RowButton(c, v.Name, v.Key() == p.selected, func() { p.Open(v.Key()) }).Fill().MinWidth(0).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
						slot = row.Children(func() {
							p.libraryListBody(c, v, data, &actions, !p.reorder.dragging && (row.Hovered() || row.FocusVisible()))
						})
					} else {
						cell := libraryCellData(v, data)
						if p.reorder.dragging {
							cell.Stats = ""
						}
						slot = kit.InstanceCell(c, cell, func() { p.Open(v.Key()) }).MinWidth(0)
						if !p.reorder.dragging && (slot.Hovered() || slot.FocusVisible()) {
							slot.Children(func() {
								ui.Box(c).Absolute().Top(8).Right(8).Children(func() { p.quickLaunch(c, v, data, actions) })
							})
						}
					}
					handle := ui.Local(slot, "reorder-slot", func() ui.Handle { return ui.Handle{} })
					slot.Bind(handle)
					p.reorder.slots = append(p.reorder.slots, instanceSlot{i, *handle})
					offset := geometry.offset(i, preview.destination(i))
					var from, to ui.Rect
					for _, old := range oldSlots {
						if old.index == i {
							from = old.handle.Bounds(c)
						}
						if old.index == preview.destination(i) {
							to = old.handle.Bounds(c)
						}
					}
					if from.W > 0 && to.W > 0 {
						offset = ui.Point{X: to.X - from.X, Y: to.Y - from.Y}
					}
					animation := kit.LayoutAnimation(kit.MotionMove)
					slot.AnimatePose(ui.Pose{X: offset.X, Y: offset.Y}, animation).Transition(ui.ElementTransition{Position: true, Animation: &animation, Match: instanceCardMotionKey{p, v.Key(), p.reorder.generation}})
					if p.reorder.dragging && p.reorder.key == v.Key() {
						slot.Opacity(0)
					}
					if preview.key != "" && i == preview.to {
						insertionTarget = slot
					}
					p.instanceContextMenu(c, slot, v, data, actions)
				})
			}
			for n := end - start; n < cols; n++ {
				ui.Box(c).Grow(1).Basis(0).MinWidth(0)
			}
		})
	})
	p.instanceReorderInput(c, grid, installed, actions)
	if p.inline {
		grid.Focusable()
		move := func(delta int) {
			if len(installed) == 0 {
				return
			}
			index := 0
			for i, v := range installed {
				if v.Key() == p.selected {
					index = i
					break
				}
			}
			index = max(0, min(len(installed)-1, index+delta))
			p.Open(installed[index].Key())
			p.grid.Focus(c)
			p.libraryRows.ScrollTo(index, ui.Center)
		}
		grid.OnShortcut(0, ui.KeyUp, func() { move(-1) }).OnShortcut(0, ui.KeyDown, func() { move(1) }).OnShortcut(0, ui.KeyEnter, func() {})
	}
	if insertionTarget.Valid() {
		bounds := insertionTarget.Bounds()
		grid.Children(func() { c.Key("insertion-preview"); kit.InsertionPlaceholder(c, insertionTarget, bounds.W, bounds.H) })
	}
	p.instanceDragPreview(c, installed, data)
}

func libraryCellData(v minecraft.Instance, data InstancesData) kit.CellData {
	activity := InstanceActivity{}
	if data.Activity != nil {
		activity = data.Activity(v.Key())
	}
	stat := data.Statistics[v.Key()]
	stats := locales.NotPlayedYet()
	if stat.Seconds > 0 {
		stats = fmt.Sprintf(locales.HoursTotal(), float64(stat.Seconds)/3600)
		if stat.Known && stat.Mods > 0 {
			stats = fmt.Sprintf(locales.ModsHoursPlayed(), stat.Mods, float64(stat.Seconds)/3600)
		}
	}
	return kit.CellData{Icon: data.Icons[v.Key()], Title: v.Name, Subtitle: kit.MetaLine(v.GameVersion, v.Loader), Stats: stats, Running: activity.Running}
}

func (p *Instances) filterInstances(installed []minecraft.Instance) []minecraft.Instance {
	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" {
		return installed
	}
	source := &installed[0]
	if source != p.filterSource || len(installed) != p.filterCount || query != p.filterQuery {
		p.filterSource, p.filterCount, p.filterQuery = source, len(installed), query
		clear(p.filtered)
		p.filtered = p.filtered[:0]
		for _, instance := range installed {
			if strings.Contains(strings.ToLower(instance.Name+" "+instance.GameVersion+" "+instance.Loader+" "+instance.SourceName), query) {
				p.filtered = append(p.filtered, instance)
			}
		}
	}
	return p.filtered
}

func (p *Instances) instanceDetail(c *ui.Context, installed []minecraft.Instance, data InstancesData, actions InstancesActions) {
	panel := ui.Column(c).Grow(1).Basis(0).MinWidth(0).MinHeight(0).AlignSelf(ui.Stretch).Gap(kit.Gap(c))
	v := installed[0]
	for _, candidate := range installed {
		if candidate.Key() == p.selected {
			v = candidate
			break
		}
	}
	p.selected = v.Key()
	panel.Children(func() {
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).AlignSelf(ui.Center).Gap(kit.Gap(c)).Children(func() {
			p.instanceHeader(c, v, data, actions)
			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Clip().Children(func() {
				c.Key(v.Key() + ":" + p.tab)
				kit.SlideMotion(ui.Column(c).Grow(1).FillWidth().MinHeight(0), p.tabForward).Children(func() {
					switch p.tab {
					case "servers":
						if data.Servers != nil {
							p.servers(c, data.Servers, actions.ServerActions)
						}
					case "worlds", "screenshots":
						p.contentFiles(c, data, actions)
					case string(resource.Mod), string(resource.ResourcePack), string(resource.ShaderPack):
						if data.Resources != nil && data.Resources.Open && data.Resources.Embedded && string(data.Resources.Kind) == p.tab {
							p.resources.View(c, data.Resources, actions.ResourceActions)
						}
					default:
						p.overview(c, v, data, actions)
					}
				})
			})
		})
	})
}
