package page

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type DirectoryPickerData struct {
	ID                          uint64
	Open, Loading, Saving       bool
	Title, Path, Current, Error string
	ShowHidden                  bool
	CanBack, CanForward         bool
	Directories, Locations      []fsutil.DirectoryEntry
	Files                       bool
	Extensions, SelectedFiles   []string
	FileNames                   []string
	SingleFile                  bool
	ConfirmLabel                string
}

type DirectoryPickerActions struct {
	Navigate        func(string)
	Create          func(string)
	Confirm, Cancel func()
	Back, Forward   func()
}

type DirectoryPicker struct {
	id             uint64
	current        string
	list           ui.ListState
	visible        []int
	creating       bool
	newFolder      string
	address        string
	filter         string
	selected       int
	chosen         ui.Selection[string]
	addressHandle  ui.Handle
	editingAddress bool
	breadcrumbPath string
	filterSource   *fsutil.DirectoryEntry
	filterCount    int
	filterQuery    string
	filterHidden   bool
}

func (p *DirectoryPicker) View(c *ui.Context, data *DirectoryPickerData, actions DirectoryPickerActions) {
	if p.id != data.ID {
		*p = DirectoryPicker{id: data.ID, address: data.Path, selected: -1}
	}
	if p.current != data.Current {
		p.current, p.list, p.creating, p.newFolder = data.Current, ui.ListState{}, false, ""
		p.address, p.filter, p.selected = data.Current, "", -1
		p.editingAddress = false
		p.filterSource, p.filterCount = nil, -1
		p.chosen.Clear()
		data.SelectedFiles = nil
	}
	if !data.Open {
		return
	}
	c.OnShortcut(0, ui.KeyEscape, actions.Cancel)
	c.OnShortcut(ui.Alt, ui.KeyLeft, actions.Back)
	c.OnShortcut(ui.Alt, ui.KeyRight, actions.Forward)
	c.OnShortcut(ui.Alt, ui.KeyUp, func() {
		if !data.Saving && data.Current != "" {
			actions.Navigate(filepath.Dir(data.Current))
		}
	})
	c.OnShortcut(ui.Cmd, ui.KeyL, func() { p.editAddress(c, data) })
	ui.Column(c).Fill().Background(kit.Surface).Children(func() {
		bar := c.TitleBar()
		ui.Row(c).FillWidth().Height(max(40, bar.Height)).Shrink(0).
			Padding(0, bar.Right+16, 0, bar.Left+16).AlignItems(ui.Center).DragWindow().Children(func() {
			kit.Label(c, data.Title, kit.Body).Grow(1)
			kit.WindowButton(c, kit.Close, true, actions.Cancel).Label(locales.Close())
		})
		ui.Column(c).Grow(1).FillWidth().Padding(12, 16, 16, 16).Gap(12).Children(func() {
			p.navigation(c, data, actions)
			p.browser(c, data, actions)
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				label := locales.SelectedFolder()
				if data.Files {
					label = locales.Filename()
				}
				kit.Label(c, label, kit.Body).Width(56).TextColor(kit.Muted)
				c.Key("selected-directory")
				kit.Input(c, &data.Path).Grow(1).MinWidth(0).Label(locales.SelectedFolder()).Disabled(data.Loading || data.Saving).
					OnChange(func() { p.selected = -1; p.chosen.Clear(); data.SelectedFiles = nil })
			})
			if data.Error != "" {
				kit.ErrorText(c, data.Error)
			}
			p.footer(c, data, actions)
		})
	})
}

func (p *DirectoryPicker) navigation(c *ui.Context, data *DirectoryPickerData, actions DirectoryPickerActions) {
	ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		c.Key("back")
		kit.IconButton(c, kit.ArrowLeft, false, actions.Back).Label(locales.Back2()).Disabled(data.Saving || !data.CanBack)
		c.Key("forward")
		kit.IconButton(c, kit.ArrowRight, false, actions.Forward).Label(locales.Forward()).Disabled(data.Saving || !data.CanForward)
		c.Key("parent")
		kit.IconButton(c, kit.ArrowUp, false, func() { actions.Navigate(filepath.Dir(data.Current)) }).Label(locales.Up()).
			Disabled(data.Saving || data.Current == "" || filepath.Dir(data.Current) == data.Current)
		if p.editingAddress || data.Current == "" {
			c.Key("directory-address")
			kit.Input(c, &p.address).Bind(&p.addressHandle).Grow(1).MinWidth(0).Label(locales.CurrentLocation()).Disabled(data.Saving).
				OnSubmit(func() { actions.Navigate(p.address) })
			c.Key("go")
			kit.IconButton(c, kit.ArrowRight, false, func() { actions.Navigate(p.address) }).Label(locales.Go()).
				Disabled(data.Saving || strings.TrimSpace(p.address) == "")
		} else {
			p.breadcrumbs(c, data, actions)
			c.Key("edit-address")
			kit.IconButton(c, kit.Edit, false, func() { p.editAddress(c, data) }).Label(locales.EditPath()).Disabled(data.Saving)
		}
	})
}

func (p *DirectoryPicker) editAddress(c *ui.Context, data *DirectoryPickerData) {
	if data.Saving {
		return
	}
	p.address, p.editingAddress = data.Current, true
	p.addressHandle.Focus(c)
}

func (p *DirectoryPicker) breadcrumbs(c *ui.Context, data *DirectoryPickerData, actions DirectoryPickerActions) {
	ui.ScrollHorizontal(c).Scrollbars(false).Grow(1).MinWidth(0).Height(kit.ControlH()).Radius(kit.RadiusM).Border(1, kit.LineSoft).Children(func() {
		ui.Row(c).Shrink(0).FillHeight().Padding(0, 4).AlignItems(ui.Center).Children(func() {
			root := filepath.VolumeName(data.Current) + string(filepath.Separator)
			parts := []fsutil.DirectoryEntry{{Name: root, Path: root}}
			relative := strings.TrimPrefix(data.Current, root)
			path := root
			for name := range strings.SplitSeq(relative, string(filepath.Separator)) {
				if name == "" {
					continue
				}
				path = filepath.Join(path, name)
				parts = append(parts, fsutil.DirectoryEntry{Name: name, Path: path})
			}
			for i, part := range parts {
				if i > 0 {
					ui.Icon(c, kit.ChevronRight).FontSize(kit.IconPx(12)).TextColor(kit.Muted)
				}
				c.Key(part.Path)
				item := kit.Ghost(c, part.Name, func() { actions.Navigate(part.Path) }).Height(kit.TabH()).Padding(0, 8).Shrink(0).Disabled(data.Saving)
				if i == len(parts)-1 && p.breadcrumbPath != data.Current {
					item.ScrollIntoView()
					p.breadcrumbPath = data.Current
				}
			}
		})
	})
}

func (p *DirectoryPicker) browser(c *ui.Context, data *DirectoryPickerData, actions DirectoryPickerActions) {
	ui.Row(c).FillWidth().Grow(1).MinHeight(140).Radius(kit.RadiusL).Border(1, kit.LineSoft).Clip().Children(func() {
		ui.Scroll(c).Scrollbars(false).Width(140).Shrink(0).FillHeight().Background(kit.InsetColor).Children(func() {
			ui.Column(c).FillWidth().Shrink(0).Padding(8).Gap(4).Children(func() {
				kit.Label(c, locales.Locations(), kit.Body).TextColor(kit.Muted).Padding(8, 12)
				for _, location := range data.Locations {
					c.Key(location.Path)
					active := data.Current == location.Path
					item := kit.RowButton(c, "", active, func() { actions.Navigate(location.Path) }).FillWidth().Height(kit.ControlH()).Padding(0, 12).Gap(8).Disabled(data.Saving)
					if active {
						item.Background(kit.AccentSoft)
					} else if item.Hovered() {
						item.Background(kit.Hover)
					}
					item.Children(func() {
						icon := kit.Folder
						if location.Name == locales.Home() {
							icon = kit.Home
						}
						color := kit.Muted
						if active {
							color = kit.Accent
						}
						ui.Icon(c, icon).FontSize(kit.IconPx(16)).TextColor(color)
						kit.Label(c, location.Name, kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					})
				}
			})
		})
		ui.Column(c).Grow(1).FillHeight().MinWidth(0).Padding(8).Gap(8).Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				c.Key("directory-filter")
				placeholder := locales.FilterFolders()
				if data.Files {
					placeholder = locales.FilterFiles()
				}
				kit.Input(c, &p.filter).Grow(1).MinWidth(0).Placeholder(placeholder).Disabled(data.Saving).
					OnChange(func() { p.resetSelection(data) })
				c.Key("show-hidden")
				ui.Checkbox(c, &data.ShowHidden, locales.HiddenFolders()).Disabled(data.Saving).
					OnChange(func() { p.resetSelection(data) })
			})
			if data.Loading {
				kit.Loading(c)
				return
			}
			p.filterEntries(data)
			if len(p.visible) == 0 {
				label := locales.NoSubfolders()
				if p.filter != "" {
					label = locales.NoMatchingFolders()
				}
				ui.Column(c).Grow(1).FillWidth().Gap(8).Center().Children(func() {
					ui.Icon(c, kit.Folder).FontSize(kit.IconPx(20)).TextColor(kit.Faint)
					kit.Label(c, label, kit.Body).TextColor(kit.Muted)
				})
				return
			}
			p.list.Selected = &p.selected
			p.list.Selection = nil
			if data.Files && !data.SingleFile {
				p.list.Selection = &p.chosen
			}
			p.list.Label = func(i int) string { return data.Directories[p.visible[i]].Name }
			c.Key(data.Current)
			kit.PresenceMotion(ui.List(c, &p.list, len(p.visible)).Grow(1).FillWidth().Padding(0, 12, 0, 0)).Disabled(data.Saving).
				OnChange(func() {
					if entry, ok := p.selection(data); ok {
						data.Path = entry.Path
					}
					if data.Files {
						data.SelectedFiles = nil
						for _, entry := range data.Directories {
							if !entry.Directory && p.chosen.Has(entry.Path) {
								data.SelectedFiles = append(data.SelectedFiles, entry.Path)
							}
						}
						if len(data.SelectedFiles) > 1 {
							data.Path = strconv.Itoa(len(data.SelectedFiles)) + locales.Files2()
						}
					}
				}).OnSubmit(func() {
				if entry, ok := p.selection(data); ok {
					if entry.Directory || !data.Files {
						actions.Navigate(entry.Path)
					} else {
						actions.Confirm()
					}
				}
			}).
				ItemKey(func(i int) any { return data.Directories[p.visible[i]].Path }).Scrollbars(false).Rows(func(row ui.ListRow) {
				c := row.Context
				entry := data.Directories[p.visible[row.Index]]
				ui.Row(c).FillWidth().Height(kit.ControlH()).Padding(0, 12).Gap(12).Radius(kit.RadiusS).AlignItems(ui.Center).Children(func() {
					if entry.Directory || !data.Files {
						ui.Icon(c, kit.Folder).FontSize(kit.IconPx(16)).TextColor(kit.AccentText)
					} else {
						ui.Icon(c, kit.Cube).FontSize(kit.IconPx(16)).TextColor(kit.Muted)
					}
					kit.Label(c, entry.Name, kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				})
			})
			unit := locales.Folders()
			if data.Files {
				unit = locales.Items()
			}
			kit.Label(c, strconv.Itoa(len(p.visible))+unit, kit.Body).TextColor(kit.Muted).Padding(0, 12)
		})
	})
}

func (p *DirectoryPicker) filterEntries(data *DirectoryPickerData) {
	var source *fsutil.DirectoryEntry
	if len(data.Directories) > 0 {
		source = &data.Directories[0]
	}
	if source == p.filterSource && len(data.Directories) == p.filterCount && p.filter == p.filterQuery && data.ShowHidden == p.filterHidden {
		return
	}
	p.filterSource, p.filterCount, p.filterQuery, p.filterHidden = source, len(data.Directories), p.filter, data.ShowHidden
	query := strings.ToLower(p.filter)
	p.visible = p.visible[:0]
	for i, entry := range data.Directories {
		if !data.ShowHidden && strings.HasPrefix(entry.Name, ".") {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(entry.Name), query) {
			p.visible = append(p.visible, i)
		}
	}
}

func (p *DirectoryPicker) selection(data *DirectoryPickerData) (fsutil.DirectoryEntry, bool) {
	if p.selected < 0 || p.selected >= len(p.visible) {
		return fsutil.DirectoryEntry{}, false
	}
	return data.Directories[p.visible[p.selected]], true
}

func (p *DirectoryPicker) resetSelection(data *DirectoryPickerData) {
	p.chosen.Clear()
	data.SelectedFiles = nil
	p.list, p.selected = ui.ListState{}, -1
	data.Path = data.Current
}

func (p *DirectoryPicker) footer(c *ui.Context, data *DirectoryPickerData, actions DirectoryPickerActions) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
		if !data.Files && p.creating {
			kit.Input(c, &p.newFolder).Width(kit.W(160)).Placeholder(locales.FolderName()).Disabled(data.Loading || data.Saving).
				OnSubmit(func() { actions.Create(p.newFolder) })
			kit.Button(c, locales.Create(), func() { actions.Create(p.newFolder) }).Disabled(data.Loading || data.Saving || strings.TrimSpace(p.newFolder) == "")
			kit.IconButton(c, kit.Close, false, func() { p.creating = false }).Label(locales.CancelNewFolder())
		} else if !data.Files {
			kit.Button(c, locales.NewFolder(), func() { p.creating, p.newFolder = true, "" }).Disabled(data.Loading || data.Saving || data.Current == "")
		}
		ui.Spacer(c)
		label := locales.ChooseFolder()
		if data.Files {
			label = locales.Import()
		}
		if data.ConfirmLabel != "" {
			label = data.ConfirmLabel
		}
		kit.Primary(c, label, actions.Confirm).Disabled(data.Loading || data.Saving || strings.TrimSpace(data.Path) == "" || (data.Files && len(data.SelectedFiles) == 0 && data.Path == data.Current))
	})
}
