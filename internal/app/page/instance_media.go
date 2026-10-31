package page

import (
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func mediaImage(c *ui.Context, file instancecontent.File, data InstancesData, actions InstancesActions, world bool) {
	holder := ui.Box(c).Fill()
	if holder.InView() && actions.LoadThumbnail != nil {
		actions.LoadThumbnail(file)
	}
	if world && file.Path != "" && holder.InView() && actions.LoadWorld != nil {
		actions.LoadWorld(file)
	}
	checked := ui.Local(holder, "media-checked", func() bool { return false })
	if !*checked {
		*checked = true
		c.After(time.Millisecond)
	}
	holder.Children(func() {
		if bitmap := data.Content.Thumbnails[file.Cover]; bitmap != nil {
			ui.Image(c, bitmap).Fill().Fit(ui.Cover).ContentTransition(kit.ImageMotion)
		} else {
			ui.Box(c).Fill().Center().Background(kit.AccentSoft).Children(func() {
				icon := kit.Image
				if world {
					icon = kit.Cube
				}
				ui.Icon(c, icon).FontSize(kit.IconPx(20)).TextColor(kit.AccentText)
			})
		}
	})
}

func mediaViewport(c *ui.Context, el ui.Element) {
	scroll := ui.Local(el, "media-scroll", func() ui.ScrollState { return ui.ScrollState{} })
	el.TrackScroll(scroll)
	previous := ui.Local(el, "media-offset", func() ui.Point { return ui.Point{X: -1, Y: -1} })
	position := ui.Point{X: scroll.X, Y: scroll.Y}
	if position != *previous {
		*previous = position
		c.After(time.Millisecond)
	}
}

func (p *Instances) contentFiles(c *ui.Context, data InstancesData, actions InstancesActions) {
	files, label, subdir := data.Content.Summary.Worlds, locales.Worlds(), "saves"
	world := p.tab == "worlds"
	if !world {
		files, label, subdir = data.Content.Summary.Screenshots, locales.Screenshots(), "screenshots"
	}
	kit.Toolbar(c, func() {
		kit.Label(c, fmt.Sprintf(locales.Message(), len(files), label), kit.Caption).Grow(1)
		if world {
			p.worldImportMenu(c, data, actions)
		}
		kit.IconButton(c, kit.Refresh, false, actions.RefreshContent).Label(locales.Refresh()).Tooltip(locales.Refresh()).Disabled(data.Content.Loading)
		kit.IconButton(c, kit.Folder, false, func() { actions.Open(filepath.Join(data.Content.Root, subdir)) }).Label(locales.OpenFolder()).Tooltip(locales.OpenFolder())
	})
	if data.Content.Error != "" {
		kit.ErrorText(c, data.Content.Error)
	}
	if data.Content.Loading {
		kit.Loading(c)
		return
	}
	if len(files) == 0 {
		icon := kit.Image
		if world {
			icon = kit.Cube
		}
		kit.EmptyState(c, icon, locales.No()+label, "", nil)
		return
	}
	if !world {
		p.gallery.Key = func(i int) any { return files[i].Path }
		p.gallery.Label = func(i int) string { return files[i].Name }
		width := p.gallery.Bounds(c).W
		cols := max(1, int((width+8)/(160+8)))
		height := max(90, (width-float32(cols-1)*8)/float32(cols)*9/16)
		if width == 0 {
			c.After(0)
		}
		grid := ui.GridView(c, &p.gallery, len(files), 160, height, func(i int) {
			file := files[i]
			button := ui.ButtonBase(c).Fill().Radius(kit.RadiusM).Clip().OnClick(func() { actions.Preview(file.Path) }).Label(locales.PreviewScreenshot() + file.Name)
			button.Children(func() {
				mediaImage(c, file, data, actions, false)
				if button.Hovered() || button.FocusVisible() {
					ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Padding(16, 8, 8, 8).Draw(func(p *ui.Painter, r ui.Rect) {
						p.FillGradient(r, ui.LinearGradient{Angle: 180, From: ui.Transparent, To: kit.BackdropStrong}, 0)
					}).Children(func() {
						kit.Line(c, file.Modified.Format("2006-01-02 15:04"), kit.Caption).TextColor(kit.ImageInk)
					})
				}
			})
			screenshotContext(c, button, file, data.Busy, actions)
		}).Grow(1).FillWidth()
		kit.PresenceMotion(grid)
		mediaViewport(c, grid)
		return
	}
	c.Key("world-results")
	list := kit.PresenceMotion(ui.List(c, &p.files, len(files))).Grow(1).FillWidth().MinHeight(0).Clip().ItemKey(func(i int) any { return files[i].Path }).Scrollbars(false).Rows(func(row ui.ListRow) {
		c, file := row.Context, files[row.Index]
		world := data.Content.Worlds[file.Path]
		name, modified := file.Name, file.Modified
		if world.Name != "" {
			name = world.Name
		}
		if !world.LastPlayed.IsZero() {
			modified = world.LastPlayed
		}
		worldRow := kit.RowButton(c, name, false, func() { p.worldPath = file.Path }).FillWidth().Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
		if worldRow.InView() && actions.LoadWorld != nil {
			actions.LoadWorld(file)
		}
		worldRow.Children(func() {
			kit.IconBadge(c, kit.Folder, 32)
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				kit.Line(c, name, kit.Strong)
				meta := world.Version
				if world.Valid {
					meta = kit.MetaLine(meta, worldMode(world))
				}
				kit.Line(c, meta, kit.Caption)
			})
			ui.Row(c).Width(kit.W(104)).Shrink(0).Gap(8).AlignItems(ui.Center).Justify(ui.End).Children(func() {
				if worldRow.Hovered() || worldRow.FocusVisible() {
					kit.ContextMenuButton(c, worldRow, locales.WorldActions())
				} else {
					kit.Line(c, modified.Format("01-02 15:04"), kit.Caption)
				}
			})
		})
		kit.ContextMenu(c, worldRow, func(menu *kit.ContextActions) {
			menu.Item(c, "details", locales.WorldDetails(), false, func() { p.worldPath = file.Path })
			if world.HasSeed {
				services := c.Services()
				menu.Item(c, "seed", locales.CopySeed(), false, func() { services.WriteClipboard(strconv.FormatInt(world.Seed, 10)) })
			}
			menu.Item(c, "directory", locales.OpenFolder(), false, func() { actions.Open(file.Path) })
			menu.Separator(c)
			for _, option := range worldEditOptions(world, data.Busy || data.InstanceBusy) {
				if option.mode == "delete" {
					menu.Separator(c)
				}
				menu.Item(c, option.mode, option.label, option.disabled, func() { p.editWorld(file, option.mode, actions) })
			}
		})
	})
	mediaViewport(c, list)
}

func worldMode(world instancecontent.World) string {
	if world.Hardcore {
		return locales.Hardcore()
	}
	switch world.Mode {
	case 1:
		return locales.Creative()
	case 2:
		return locales.Adventure()
	case 3:
		return locales.Spectator()
	default:
		return locales.Survival()
	}
}

func (p *Instances) worldPreview(c *ui.Context, data InstancesData, actions InstancesActions) {
	if p.worldPath == "" {
		return
	}
	var file instancecontent.File
	for _, candidate := range data.Content.Summary.Worlds {
		if candidate.Path == p.worldPath {
			file = candidate
			break
		}
	}
	if file.Path == "" {
		p.worldPath = ""
		return
	}
	world, loaded := data.Content.Worlds[file.Path]
	name := file.Name
	if world.Name != "" {
		name = world.Name
	}
	open, close := true, func() { p.worldPath = "" }
	dialog := kit.Dialog(c, &open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, locales.WorldDetails(), close)
			ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(64, 64).Radius(kit.RadiusM).Clip().Children(func() { mediaImage(c, file, data, actions, true) })
				ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
					kit.Label(c, name, kit.Body).MaxLines(2).Ellipsis("…").Wrap()
					kit.Label(c, file.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				})
			})
			if !loaded {
				ui.Box(c).FillWidth().Height(80).Center().Children(func() { ui.Spinner(c) })
			} else if world.Valid {
				kit.Group(c, locales.WorldDetails(), func() {
					kit.DetailRow(c, locales.GameVersion(), world.Version, nil)
					kit.DetailRow(c, locales.GameMode(), worldMode(world), nil)
					if world.HasSeed {
						services, seed := c.Services(), strconv.FormatInt(world.Seed, 10)
						kit.DetailRow(c, locales.Seed(), seed, func() {
							kit.Button(c, locales.Copy(), func() { services.WriteClipboard(seed) })
						})
					}
					if !world.LastPlayed.IsZero() {
						kit.DetailRow(c, locales.LastPlayed(), world.LastPlayed.Local().Format("2006-01-02 15:04"), nil)
					}
				})
			} else {
				kit.ErrorText(c, locales.CouldNotReadWorldDetails())
			}
			kit.DialogFooter(c, func() { p.worldMenu(c, file, world, data, actions) })
		})
	})
	if dialog.Dismissed() {
		close()
	}
}

func (p *Instances) screenshotPreview(c *ui.Context, data InstancesData, actions InstancesActions) {
	path := data.Content.PreviewPath
	if path == "" {
		return
	}
	files := data.Content.Summary.Screenshots
	index := -1
	for i, file := range files {
		if file.Path == path {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	file := files[index]
	step := func(offset int) {
		if next := index + offset; index >= 0 && next >= 0 && next < len(files) {
			actions.Preview(files[next].Path)
		}
	}
	open := true
	dialog := kit.Dialog(c, &open, kit.DialogViewer, func(back, panel ui.Element) {
		back.Background(kit.BackdropStrong)
		panel.Height(720).MaxHeightPercent(90)
		panel.OnShortcut(0, ui.KeyLeft, func() { step(-1) }).OnShortcut(0, ui.KeyRight, func() { step(1) }).
			OnShortcut(0, ui.KeyHome, func() { actions.Preview(files[0].Path) }).OnShortcut(0, ui.KeyEnd, func() { actions.Preview(files[len(files)-1].Path) })
		if !data.Busy {
			panel.OnShortcut(ui.Cmd, ui.KeyS, func() { actions.EditScreenshot(file, "export") }).
				OnShortcut(0, ui.KeyDelete, func() { actions.EditScreenshot(file, "delete") })
		}
		panel.Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Label(c, filepath.Base(path), kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				kit.IconButton(c, kit.Close, false, actions.ClosePreview).Label(locales.CloseScreenshotPreview())
			})
			ui.Box(c).Grow(1).FillWidth().MinHeight(0).Radius(kit.RadiusM).Clip().Background(kit.InsetColor).Children(func() {
				bitmap := data.Content.Preview
				if bitmap == nil {
					bitmap = data.Content.Thumbnails[path]
				}
				if bitmap != nil {
					ui.Image(c, bitmap).Fill().Fit(ui.Contain).ContentTransition(kit.ImageMotion)
				}
				if data.Content.PreviewLoading {
					ui.Box(c).Absolute().Right(12).Bottom(12).Padding(8).Radius(kit.RadiusM).Background(kit.Surface).Children(func() { ui.Spinner(c).Size(16, 16) })
				}
				if data.Content.PreviewError != "" {
					ui.Box(c).Fill().Center().Children(func() { kit.ErrorText(c, data.Content.PreviewError) })
				}
			})
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				kit.IconButton(c, kit.ArrowLeft, false, func() { step(-1) }).Label(locales.PreviousScreenshot()).Disabled(index <= 0)
				kit.Label(c, fmt.Sprintf("%d / %d", index+1, len(files)), kit.Caption)
				kit.IconButton(c, kit.ArrowRight, false, func() { step(1) }).Label(locales.NextScreenshot()).Disabled(index < 0 || index >= len(files)-1)
				ui.Spacer(c)
				screenshotMenu(c, file, data.Busy, actions)
			})
		})
	})
	if dialog.Dismissed() {
		actions.ClosePreview()
	}
}
