package page

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type ResourcesData struct {
	ID                                                              uint64
	Open, Online, Loading, EntriesLoading, Busy, Embedded, FullPage bool
	Instance, Root, GameVersion, Loader, Category                   string
	World                                                           string
	Kind                                                            resource.Kind
	CanInstallMods                                                  bool
	Entries                                                         []resource.Entry
	Error, Status, Query                                            string
	Source                                                          resourcecatalog.Source
	Compatibility                                                   resourcecatalog.CompatibilityFilter
	GameVersions                                                    []minecraft.VersionSummary
	Search                                                          resourcecatalog.SearchResult
	SearchOffset                                                    int
	SearchRevision                                                  uint64
	Icons                                                           map[string]*ui.Bitmap
	Animations                                                      map[string][]byte
	Videos                                                          map[string]*ui.VideoPlayer
	Project                                                         *resourcecatalog.Project
	Versions                                                        []resourcecatalog.Version
	Missing                                                         []resourcecatalog.MissingFile
	MissingOpen                                                     bool
	MissingBusy                                                     bool
	MissingError, MissingStatus                                     string
	UpdatesOpen, UpdatesLoading                                     bool
	UpdateSource                                                    resourcecatalog.Source
	UpdateCheck                                                     *resourcecatalog.UpdateCheck
	UpdateError                                                     string
	UpdateDetail                                                    *resourcecatalog.ResourceUpdate
	UpdateDetails                                                   resourcecatalog.VersionDetails
	Targets                                                         []minecraft.Instance
}

type ResourcesActions struct {
	AssessUpdates                                        func(string, []resourcecatalog.ResourceUpdate) SpaceData
	Close, Refresh, Import, OpenDirectory                func()
	ImportDirectory                                      func()
	Change                                               func(resource.Kind, bool)
	Search                                               func(string, int)
	Versions                                             func(resourcecatalog.Project)
	Back                                                 func()
	Install                                              func(resourcecatalog.Version, resource.InstallPolicy, string)
	SourceChanged, CategoryChanged                       func(string)
	Toggle                                               func([]resource.Entry, bool)
	Remove                                               func([]resource.Entry)
	Origin                                               func(resource.Entry, resource.Origin)
	EnsureVideo                                          func(string)
	ReleaseVideos                                        func(map[string]struct{})
	OpenUpdates, CloseUpdates, CheckUpdates, BackUpdates func()
	Update                                               func([]resourcecatalog.ResourceUpdate)
	UpdateDetails                                        func(resourcecatalog.ResourceUpdate)
	Supplement, RetryMissing, CloseMissing               func()
}

type Resources struct {
	toolbarAction          string
	identity               resourceIdentity
	query                  string
	localStatus, localSort string
	nameChoice             string
	choiceQuery            string

	list                    ui.ListState
	onlineList, versionList ui.ListState
	searchRevision          uint64
	missingList             ui.ListState
	selected                ui.Selection[string]
	focusList               bool
	selectionAnchor         string
	selectionDrag           resourceSelectionDrag
	pending                 []resource.Entry
	confirm                 bool
	filterSource            *resource.Entry
	filterCount             int
	filterQuery             string
	filtered                []resource.Entry
	updateCheck             *resourcecatalog.UpdateCheck
	updateList              ui.ListState
	updateSelected          ui.Selection[string]
	updateSpaceDirty        bool
	updateSpaceVersion      uint64
	updateSpaceItems        []resourcecatalog.ResourceUpdate
	updateSpaceKey          string
	conflict                bool
	pendingInstall          *resourcecatalog.Version
	conflictEntries         []resource.Entry
	conflictChoice          int
	targetOpen              bool
	pendingTarget           *resourcecatalog.Version
	pendingTargetPolicy     resource.InstallPolicy
	versionCompatibility    resourcecatalog.CompatibilityFilter
	versionChannel          string
	projectID               string
}

type resourceIdentity struct {
	id     uint64
	root   string
	kind   resource.Kind
	online bool
	source resourcecatalog.Source
}

func (p *Resources) View(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if !data.Open {
		return
	}
	identity := resourceIdentity{id: data.ID, root: data.Root, kind: data.Kind, online: data.Online, source: data.Source}
	if identity != p.identity {
		p.identity, p.query = identity, data.Query
		p.nameChoice = ""
		p.list, p.selected, p.pending, p.confirm = ui.ListState{}, ui.Selection[string]{}, nil, false
		p.onlineList, p.versionList = ui.ListState{}, ui.ListState{}
		p.conflict, p.pendingInstall, p.conflictEntries = false, nil, nil
		p.targetOpen, p.pendingTarget = false, nil
		p.versionCompatibility, p.versionChannel = data.Compatibility, ""
		p.filterSource, p.filtered, p.filterCount = nil, nil, 0
		p.selectionAnchor, p.selectionDrag = "", resourceSelectionDrag{}
	}
	if p.choiceQuery != data.Query {
		p.choiceQuery, p.nameChoice = data.Query, ""
	}
	if data.Online && p.searchRevision != data.SearchRevision {
		p.searchRevision, p.onlineList = data.SearchRevision, ui.ListState{}
	}
	if data.Project == nil {
		p.projectID = ""
	} else if p.projectID != data.Project.ID {
		p.projectID = data.Project.ID
		p.versionCompatibility, p.versionChannel = data.Compatibility, ""
		p.versionList, p.conflictChoice = ui.ListState{}, 0
	}
	body := func() {
		if !data.Embedded && !data.FullPage {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Label(c, data.Instance, kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				kit.Label(c, locales.Minecraft()+data.GameVersion, kit.Caption)
				if data.Loader != "" {
					kit.Pill(c, data.Loader, kit.Dim)
				}
				kit.IconButton(c, kit.Close, false, actions.Close).Label(locales.CloseResourceManager())
			})
		}
		if data.Embedded && !data.Online {
			p.localToolbar(c, data, actions, true)
		} else if !data.FullPage {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if data.World != "" {
					kit.Label(c, data.Kind.Label(), kit.Body)
				} else if !data.Embedded && !data.FullPage {
					for _, kind := range []resource.Kind{resource.Mod, resource.ResourcePack, resource.ShaderPack} {
						c.Key(kind)
						kit.NavPill(c, nil, kind.Label(), data.Kind == kind, func() { actions.Change(kind, data.Online) })
					}
				} else {
					kit.Label(c, data.Kind.Label(), kit.Body)
				}
				ui.Spacer(c)
				kit.NavPill(c, nil, locales.Local(), !data.Online, func() { actions.Change(data.Kind, false) })
				kit.NavPill(c, nil, locales.Online(), data.Online, func() { actions.Change(data.Kind, true) })
			})
		}
		if data.Error != "" && !data.Online {
			kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
		}
		project := ""
		if data.Project != nil {
			project = data.Project.ID
			c.OnShortcut(0, ui.KeyEscape, actions.Back)
		}
		kit.SlideMotion(ui.Column(c.Key("resource-content:"+project)).Grow(1).FillWidth().MinHeight(0).Gap(8), data.Project != nil).Children(func() {
			if data.Online {
				p.online(c, data, actions)
			} else {
				p.local(c, data, actions)
			}
		})
		if data.Status != "" {
			kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
		}
	}
	if data.Embedded || data.FullPage {
		kit.ContentMotion(c, ui.Column(c.Key(identity)).Grow(1).FillWidth().MinHeight(0).Gap(8)).Children(body)
	} else {
		kit.Dialog(c, &data.Open, kit.DialogL, func(back, panel ui.Element) {
			panel.Height(560)
			panel.Children(body)
		})
	}
	if p.confirm {
		kit.Dialog(c, &p.confirm, kit.DialogS, func(back, panel ui.Element) {
			panel.Children(func() {
				kit.DialogHeader(c, fmt.Sprintf(locales.DeleteSelectedItems(), len(p.pending)), func() { p.confirm = false })
				if len(p.pending) == 1 {
					kit.Label(c, p.pending[0].File, kit.Body).TextColor(kit.Muted).Wrap()
				}
				kit.DialogFooter(c, func() {
					kit.Danger(c, locales.Delete(), func() { p.confirm = false; actions.Remove(p.pending) }).Label(locales.ConfirmResourceDeletion()).Disabled(data.Busy)
				})
			})
		})
	}
	if p.conflict {
		p.conflictDialog(c, data, actions)
	}
	if p.targetOpen {
		p.targetDialog(c, data, actions)
	}
	if !data.Open {
		actions.Close()
	}
	p.missing(c, data, actions)
	p.updates(c, data, actions)
}

func (p *Resources) local(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if !data.Embedded {
		p.localToolbar(c, data, actions, false)
	}
	visible := p.filterEntries(data.Entries)
	count := 0
	if p.selected.Len() > 0 {
		for _, entry := range visible {
			if p.selected.Has(entry.Relative) {
				count++
			}
		}
	}
	selectionBar := func() {
		ui.Row(c).FillWidth().Gap(8).MinHeight(32).Padding(8, 16).AlignItems(ui.Center).Radius(kit.RadiusS).Background(kit.InsetColor).Children(func() {
			kit.SelectionBox(c, count > 0 && count == len(visible), count > 0 && count < len(visible), locales.SelectAllResources(), func() {
				for _, entry := range visible {
					if count == len(visible) {
						p.selected.Remove(entry.Relative)
					} else {
						p.selected.Add(entry.Relative)
					}
				}
				p.focusList = true
			}).Disabled(data.Loading || len(visible) == 0)
			label := fmt.Sprintf(locales.Items2(), len(visible))
			if count > 0 {
				label = fmt.Sprintf(locales.Selected3(), count)
			}
			kit.Line(c, label, kit.Caption).Grow(1).ContentTransition(kit.NumberMotion)
			if count > 0 {
				kit.FoldMotion(ui.Row(c.Key("selection-actions")).Gap(8)).Children(func() {
					kit.Button(c, locales.Enable(), func() { actions.Toggle(p.selectedEntries(visible), true) }).Disabled(data.Busy)
					kit.Button(c, locales.Disable(), func() { actions.Toggle(p.selectedEntries(visible), false) }).Disabled(data.Busy)
					kit.Button(c, locales.Delete(), func() { p.pending, p.confirm = p.selectedEntries(visible), true }).Disabled(data.Busy)
				})
			} else if actions.OpenUpdates != nil {
				kit.Button(c, locales.CheckForUpdates(), actions.OpenUpdates).Disabled(data.Busy || data.Loading)
			}
			if count == 0 {
				order := map[string]string{"": locales.Name(), "file": locales.Filename(), "size": locales.Size()}[p.localSort]
				kit.FilterSelect(c, &p.localSort, locales.Sort()+" · "+order, []kit.SelectOption[string]{{Value: "", Label: locales.Name()}, {Value: "file", Label: locales.Filename()}, {Value: "size", Label: locales.Size()}}, 160, nil)
			}
		})
	}
	area := ui.Column(c).Grow(1).FillWidth().MinHeight(0).Clip()
	area.OnShortcut(0, ui.KeyEscape, func() { p.selected.Clear() })
	area.Children(func() {
		selectionBar()
		if data.Loading {
			kit.Loading(c)
			return
		}
		if len(visible) == 0 {
			kit.EmptyState(c, nil, locales.No()+data.Kind.Label(), "", nil)
			return
		}
		p.list.Selection = &p.selected
		p.list.MenuKeepsSelection = true
		p.list.Label = func(i int) string { return visible[i].Name }
		theme := c.Theme()
		listTheme := *theme
		listTheme.SurfaceHover, listTheme.Accent = ui.Transparent, ui.Transparent
		listTheme.AccentText = kit.Text
		c.SetTheme(&listTheme)
		defer c.SetTheme(theme)
		dragSeen := false
		c.Key("resource-results")
		list := kit.PresenceMotion(ui.List(c, &p.list, len(visible)).Grow(1).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0)).
			ItemKey(func(i int) any { return visible[i].Relative }).Scrollbars(false).Rows(func(row ui.ListRow) {
			c := row.Context
			c.SetTheme(theme)
			defer c.SetTheme(&listTheme)
			entry := visible[row.Index]
			content := kit.RowButton(c, entry.DisplayName(), false, nil).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
			p.resourceRowInput(c, content, row.Index, visible, &dragSeen)
			kit.ListRowBackground(c, content, p.selected.Has(entry.Relative))
			content.Children(func() {
				ui.Box(c).Size(32, 32).Shrink(0).Center().Children(func() {
					if content.Hovered() || p.selected.Has(entry.Relative) {
						var mods ui.Modifiers
						check := kit.SelectionBox(c, p.selected.Has(entry.Relative), false, locales.SelectResource()+entry.File, func() { p.selectResource(visible, row.Index, mods, true) })
						mods = check.ClickModifiers()
					} else {
						kit.IconBadge(c, kit.Cube, 32)
					}
				})
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
						color := kit.Text
						if !entry.Enabled {
							color = kit.Muted
						}
						kit.Line(c, entry.DisplayName(), kit.Strong).TextColor(color)
						if !entry.Enabled {
							kit.Badge(c, locales.Disabled2(), kit.Dim)
						}
						if entry.Version != "" {
							kit.Line(c, entry.Version, kit.Caption).MaxWidth(96)
						}
					})
					line := kit.Line(c, entry.File, kit.Caption)
					if entry.Problem != "" {
						line.TextColor(kit.Red).Tooltip(entry.Problem)
					}
				})
				ui.Row(c).Width(kit.W(80)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
					if content.Hovered() || content.FocusVisible() {
						kit.ContextMenuButton(c, content, locales.MoreResourceActions())
					} else {
						ui.Box(c).Size(28, 28)
					}
					enabled := entry.Enabled
					kit.Switch(c, &enabled).Label(locales.Enable2() + entry.File).Disabled(data.Busy).OnChange(func() { actions.Toggle([]resource.Entry{entry}, !entry.Enabled) })
				})
			})
			kit.ContextMenu(c, content, func(menu *kit.ContextActions) {
				entries := func() []resource.Entry {
					if p.selected.Has(entry.Relative) {
						return p.selectedEntries(visible)
					}
					return []resource.Entry{entry}
				}
				menu.Item(c, "enable", locales.Enable(), data.Busy, func() { actions.Toggle(entries(), true) })
				menu.Item(c, "disable", locales.Disable(), data.Busy, func() { actions.Toggle(entries(), false) })
				for _, origin := range entry.Origins {
					menu.Item(c, "origin:"+origin.Source, locales.ViewResourceOrigin()+resourcecatalog.Source(origin.Source).Label(), false, func() { actions.Origin(entry, origin) })
				}
				menu.Separator(c)
				menu.Item(c, "delete", locales.Delete(), data.Busy, func() { p.pending, p.confirm = entries(), true })
			})
		})
		p.finishResourceDrag(c, list, visible, dragSeen)
		if p.focusList {
			p.list.Focus(c)
			p.focusList = false
		}
	})
}

func (p *Resources) localToolbar(c *ui.Context, data *ResourcesData, actions ResourcesActions, compact bool) {
	if compact && data.World == "" {
		p.libraryToolbar(c, data, actions)
		return
	}
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		kit.SearchInput(c, &p.query, locales.SearchLocalResources()).Grow(1).MinWidth(0)
		if data.Kind == resource.DataPack {
			c.Key("data-pack-import")
			holder := ui.Row(c).Shrink(0)
			choice := ui.Local(holder, "import-mode", func() string { return "" })
			holder.Children(func() {
				menu := kit.SelectBase(c, choice)
				kit.MenuButton(c, menu.Trigger, locales.Import(), menu.Open()).Disabled(data.Busy).OnChange(func() {
					mode := *choice
					*choice = ""
					if mode == "directory" {
						actions.ImportDirectory()
					} else {
						actions.Import()
					}
				})
				menu.Popup(func(panel ui.Element) {
					kit.PopupPanel(c, panel, 144, menu.Trigger).Role(ui.RoleMenu)
					kit.MenuItem(c, menu.Item("file"), locales.Archive())
					kit.MenuItem(c, menu.Item("directory"), locales.Folder())
				})
			})
		} else {
			kit.Button(c, locales.Import(), actions.Import).Disabled(data.Busy)
		}
		if compact {
			kit.IconButton(c, kit.Folder, false, actions.OpenDirectory).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
			kit.IconButton(c, kit.Refresh, false, actions.Refresh).Label(locales.Refresh()).Tooltip(locales.Refresh()).Disabled(data.Loading)
			kit.NavPill(c, nil, locales.Local(), true, func() { actions.Change(data.Kind, false) })
			kit.NavPill(c, nil, locales.Online(), false, func() { actions.Change(data.Kind, true) })
		} else {
			kit.Button(c, locales.OpenFolder(), actions.OpenDirectory)
			kit.Button(c, locales.Refresh(), actions.Refresh).Disabled(data.Loading)
		}
	})
}

func (p *Resources) libraryToolbar(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	kit.Toolbar(c, func() {
		kit.SearchInput(c, &p.query, locales.SearchLocalResources()).Grow(1).Basis(160).MinWidth(0)
		status := map[string]string{"": locales.All(), "enabled": locales.Enabled2(), "disabled": locales.Disabled2()}[p.localStatus]
		kit.FilterSelect(c, &p.localStatus, locales.Status()+" · "+status, []kit.SelectOption[string]{{Value: "", Label: locales.All()}, {Value: "enabled", Label: locales.Enabled2()}, {Value: "disabled", Label: locales.Disabled2()}}, 160, nil)
		kit.Button(c, locales.DownloadsPage()+data.Kind.Label(), func() { actions.Change(data.Kind, true) }).Disabled(data.Busy)

		menu := kit.SelectBase(c, &p.toolbarAction)
		trigger := menu.Trigger.Size(28, 28).Shrink(0).Radius(kit.RadiusS).Center().Label(locales.MoreResourceActions())
		if trigger.Hovered() {
			trigger.Background(kit.Hover)
		}
		trigger.Children(func() { ui.Icon(c, kit.More).FontSize(16).TextColor(kit.Muted) })
		trigger.OnChange(func() {
			action := p.toolbarAction
			p.toolbarAction = ""
			switch action {
			case "import":
				if actions.Import != nil {
					actions.Import()
				}
			case "open":
				if actions.OpenDirectory != nil {
					actions.OpenDirectory()
				}
			case "refresh":
				if actions.Refresh != nil {
					actions.Refresh()
				}
			}
		})
		menu.Popup(func(panel ui.Element) {
			kit.PopupPanel(c, panel, 144, menu.Trigger)
			kit.MenuItem(c, menu.Item("import").Disabled(data.Busy), locales.Import())
			kit.MenuItem(c, menu.Item("open"), locales.OpenFolder())
			kit.MenuItem(c, menu.Item("refresh").Disabled(data.Loading), locales.Refresh())
		})
	})
}

func (p *Resources) selectedEntries(entries []resource.Entry) []resource.Entry {
	chosen := make([]resource.Entry, 0, min(p.selected.Len(), len(entries)))
	for _, entry := range entries {
		if p.selected.Has(entry.Relative) {
			chosen = append(chosen, entry)
		}
	}
	return chosen
}

func (p *Resources) filterEntries(entries []resource.Entry) []resource.Entry {
	query := strings.ToLower(strings.TrimSpace(p.query))
	if query == "" && p.localStatus == "" && p.localSort == "" {
		return entries
	}
	var source *resource.Entry
	if len(entries) > 0 {
		source = &entries[0]
	}
	cacheKey := query + "\x00" + p.localStatus + "\x00" + p.localSort
	if source != p.filterSource || len(entries) != p.filterCount || cacheKey != p.filterQuery {
		p.filterSource, p.filterCount, p.filterQuery = source, len(entries), cacheKey
		clear(p.filtered)
		p.filtered = p.filtered[:0]
		for _, entry := range entries {
			if p.localStatus == "enabled" && !entry.Enabled || p.localStatus == "disabled" && entry.Enabled {
				continue
			}
			if strings.Contains(strings.ToLower(entry.Name+" "+entry.File+" "+entry.ID+" "+entry.TranslatedName+" "+entry.SearchAliases), query) {
				p.filtered = append(p.filtered, entry)
			}
		}
		slices.SortStableFunc(p.filtered, func(a, b resource.Entry) int {
			switch p.localSort {
			case "file":
				return strings.Compare(a.File, b.File)
			case "size":
				if a.Size > b.Size {
					return -1
				}
				if a.Size < b.Size {
					return 1
				}
				return 0
			}
			return strings.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName()))
		})
	}
	return p.filtered
}
