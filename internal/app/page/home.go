package page

import (
	"fmt"
	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/ui"
	"time"
)

type HomeData struct {
	Current                                        minecraft.Instance
	Instances                                      []minecraft.Instance
	Icons                                          map[string]*ui.Bitmap
	Recent                                         []HomeRecent
	Statistics                                     HomeStatistics
	Usage                                          HomeUsage
	Compact, Loading                               bool
	Account                                        account.Account
	Avatar                                         *ui.Bitmap
	Accounts                                       []kit.AccountItem
	Busy, Launching, Running, Finishing, Canceling bool
	Status, BusyStatus, Failure, OtherRunning      string
	FailureAction                                  string
	Since, OtherSince                              time.Time
	Progress                                       float64
	Activity                                       func(string) InstanceActivity
}

type HomeUsage struct {
	CPUTotal, CPUGame, CPULauncher                   float64
	MemUsedMiB, MemGameMiB, MemTotalMiB, LauncherMiB int
	Ready                                            bool
}

type HomeActions struct {
	Select                                                                                          func(string)
	SelectAccount                                                                                   func(string)
	OpenLibrary, OpenDirectory, OpenDetails, AddAccount, ManageAccounts, Launch, Stop, New, ViewLog func()
	OpenRecent, LaunchRecent                                                                        func(HomeRecent)
	LoadIcon                                                                                        func(minecraft.Instance)
	LaunchInstance                                                                                  func(string)
	Recover                                                                                         func()
	Menu                                                                                            func(*ui.Context, ui.Element)
}

type HomeState struct {
	Switching     bool
	query         string
	targetQuery   string
	target, shown int
	search        ui.Handle
	switchGrid    ui.Handle
	switchScroll  ui.ScrollState
}

func (p *HomeState) CloseSwitcher() { p.Switching, p.query, p.target = false, "", -1 }

func (p *HomeState) View(c *ui.Context, data HomeData, actions HomeActions) {
	data.Compact = kit.CompactWidth(c)
	pad, gap := kit.Pad(c), kit.Gap(c)
	ui.Column(c).Fill().MinWidth(0).MinHeight(0).Children(func() {
		ui.Box(c).FillWidth().Padding(pad, pad, 0, pad).Shrink(0).Children(func() {
			v := data.Current
			if actions.LoadIcon != nil && v.Name != "" {
				actions.LoadIcon(v)
			}
			name := v.Name
			if name == "" {
				name = locales.ChooseAnInstance()
			}
			kit.IdentityHeader(c, kit.Identity{Icon: data.Icons[v.Key()], Name: name, Meta: kit.MetaLine(v.GameVersion, v.Loader, v.SourceName), Switch: true, Open: p.Switching}, func() { p.toggle(c) }, func() {
				if v.Name != "" {
					kit.IconButton(c, kit.Folder, false, actions.OpenDirectory).Label(locales.OpenInstanceFolder()).Tooltip(locales.OpenInstanceFolder())
					if actions.Menu != nil {
						target := ui.Box(c).Size(32, 32).Shrink(0)
						target.Children(func() { kit.ContextMenuButton(c, target, locales.MoreInstanceActions()) })
						actions.Menu(c, target)
					}
				}
			})
		})
		body := ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(gap, pad, gap, pad)
		body.Children(func() {
			c.Key(p.Switching)
			kit.Crossfade(ui.Column(c).Fill().MinWidth(0).MinHeight(0)).Children(func() {
				if p.Switching {
					p.switchPanel(c, data, actions)
					return
				}
				ui.Scroll(c).Scrollbars(false).Fill().MinHeight(0).Children(func() {
					ui.Row(c).FillWidth().Wrap().Gap(kit.ColGap(c)).AlignItems(ui.Start).Children(func() {
						ui.Column(c).Grow(3).Basis(kit.W(240)).MinWidth(0).Shrink(0).Children(func() { p.recent(c, data, actions) })
						minWidth, maxWidth := kit.W(220), kit.W(300)
						if data.Compact {
							minWidth, maxWidth = kit.W(200), kit.W(240)
						}
						ui.Column(c).Grow(2).Basis(minWidth).MinWidth(0).MaxWidth(maxWidth).Shrink(0).Gap(24).Children(func() {
							ui.Column(c).FillWidth().Children(func() { homeStatistics(c, data) })
							ui.Column(c).FillWidth().Children(func() { homeUsage(c, data.Usage) })
						})
					})
				})
			})
		})
		p.footer(c, data, actions)
	})
}

func (p *HomeState) toggle(c *ui.Context) {
	p.Switching = !p.Switching
	p.query, p.target, p.shown = "", -1, -1
	if p.Switching {
		p.search.Focus(c)
	}
}

func (p *HomeState) switchPanel(c *ui.Context, data HomeData, actions HomeActions) {
	selectInstance := func(v minecraft.Instance, launch bool) {
		p.CloseSwitcher()
		if actions.Select != nil {
			actions.Select(v.Key())
		}
		if launch && actions.LaunchInstance != nil {
			actions.LaunchInstance(v.Key())
		}
	}
	if p.targetQuery != p.query {
		p.targetQuery, p.target, p.shown = p.query, 0, -1
		if p.query == "" {
			p.target = -1
		}
		p.switchScroll = ui.ScrollState{}
	}
	var matches []minecraft.Instance
	for _, instance := range data.Instances {
		if MatchInstance(instance, p.query) {
			matches = append(matches, instance)
		}
	}
	p.target = max(-1, min(p.target, len(matches)-1))
	choose := func() {
		if p.target >= 0 && p.target < len(matches) {
			selectInstance(matches[p.target], false)
		}
	}
	ui.Row(c).FillWidth().Height(kit.ControlH()).Gap(8).Shrink(0).Children(func() {
		kit.SearchInput(c, &p.query, locales.SearchInstances()).Bind(&p.search).Grow(1).MaxWidth(320).OnSubmit(choose)
		ui.Spacer(c)
		kit.Ghost(c, locales.ManageInstance(), actions.OpenLibrary)
		kit.IconLabel(c, kit.Plus, locales.New(), actions.New)
	})
	c.OnShortcut(0, ui.KeyEscape, func() {
		if p.query != "" {
			p.query, p.target, p.shown = "", -1, -1
		} else {
			p.CloseSwitcher()
		}
	})
	cols := max(1, int((p.switchGrid.Bounds(c).W+8)/(kit.W(kit.TileMinWidth)+8)))
	for _, move := range []struct {
		key   ui.Key
		delta int
	}{{ui.KeyLeft, -1}, {ui.KeyRight, 1}, {ui.KeyUp, -cols}, {ui.KeyDown, cols}} {
		c.OnShortcut(0, move.key, func() {
			if p.target < 0 {
				p.target = 0
			} else {
				p.target = max(0, min(len(matches)-1, p.target+move.delta))
			}
		})
	}
	c.OnShortcut(0, ui.KeyEnter, choose)
	if len(matches) == 0 {
		title, hint := locales.NoMatchingInstances(), locales.TryAnotherNameVersionOrLoader()
		var action func()
		if len(data.Instances) == 0 {
			title, hint = locales.NoGameInstances(), locales.CreateAnInstanceOrImportAModpackToGetStarted()
			action = func() { kit.Button(c, locales.New(), actions.New) }
		}
		kit.EmptyState(c, kit.Cube, title, hint, action)
		return
	}
	// Scroll content extends far enough to retain the tile's bleed and focus ring.
	ui.Scroll(c).Scrollbars(false).Bind(&p.switchGrid).TrackScroll(&p.switchScroll).Grow(1).AlignSelf(ui.Stretch).MinHeight(0).Margin(12, -12, 0, -12).Padding(0, 4).Children(func() { // uilint:ignore U05 C-SWITCH-7 requires 12px scroll bleed
		ui.Column(c).FillWidth().Padding(4, 0, kit.Gap(c), 0).Gap(8).Children(func() {
			for start := 0; start < len(matches); start += cols {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					for j := range cols {
						i := start + j
						if i >= len(matches) {
							ui.Box(c).Grow(1).Basis(0).MinWidth(0)
							continue
						}
						v := matches[i]
						c.Key(v.Key())
						ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(kit.H(56, kit.Strong, kit.Caption)).Children(func() {
							if actions.LoadIcon != nil {
								actions.LoadIcon(v)
							}
							activity := InstanceActivity{}
							if data.Activity != nil {
								activity = data.Activity(v.Key())
							}
							tile := kit.Tile(c, kit.TileData{Icon: data.Icons[v.Key()], Title: v.Name, Subtitle: kit.MetaLine(v.GameVersion, v.Loader), Current: v.Key() == data.Current.Key(), Running: activity.Running, Target: i == p.target, Disabled: data.Busy || activity.Active || !v.CanLaunch()}, func() { selectInstance(v, false) }, func() { selectInstance(v, true) }).FillWidth().MinWidth(0)
							if p.target >= 0 && i == p.target && p.shown != i {
								tile.ScrollIntoView()
								p.shown = i
							}
						})
					}
				})
			}
		})
	})
}

func (p *HomeState) footer(c *ui.Context, data HomeData, actions HomeActions) {
	ui.Row(c).FillWidth().Height(kit.H(64, kit.Strong, kit.Caption)).Shrink(0).BorderWidth(1, 0, 0, 0).BorderColor(kit.LineSoft).Padding(0, kit.Pad(c)).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Box(c).Grow(1).MinWidth(0).Margin(0, 16, 0, 0).Children(func() { homeStatus(c, data, actions) })
		characterTile(c, data, actions)
		label, icon, face, click := locales.LaunchGame(), kit.Play, kit.LaunchPrimary, actions.Launch
		disabled := data.Busy
		switch {
		case data.Current.Name == "":
			label, icon, click, disabled = locales.ChooseAnInstance(), kit.ChevronDown, func() { p.toggle(c) }, false
		case data.Canceling:
			label, icon, face, click, disabled = locales.StopGame(), kit.Stop, kit.LaunchDanger, actions.Stop, true
		case data.Finishing:
			label, icon, face, click, disabled = locales.CancelCommand(), kit.Close, kit.LaunchSecondary, actions.Stop, false
		case data.Running:
			label, icon, face, click, disabled = locales.StopGame(), kit.Stop, kit.LaunchDanger, actions.Stop, false
		case data.Launching:
			label, icon, face, click, disabled = locales.CancelLaunch(), kit.Close, kit.LaunchSecondary, actions.Stop, false
		}
		kit.LaunchButton(c, face, icon, label, click).Disabled(disabled)
	})
}

func homeStatus(c *ui.Context, data HomeData, actions HomeActions) {
	state, detail, icon, color := locales.LastPlayed(), relativePlayTime(data.Statistics.LastPlayed), kit.Clock, kit.Muted
	spinner, dot, lastPlayed := false, false, false
	if detail == "" {
		state = locales.NotPlayedYet()
	}
	switch {
	case data.Current.Name == "":
		state, detail, icon = locales.NoInstanceSelected(), "", kit.Cube
	case data.Canceling:
		state, detail, spinner, color = locales.Stopping(), locales.WaitingForTheGameToExit(), true, kit.Accent
	case data.Finishing:
		state, detail, spinner, color = locales.RunningPostExitCommands(), data.Status, true, kit.Accent
	case data.Running:
		state, detail, dot, color = locales.Running2(), locales.RunningFor()+" "+elapsed(data.Since), true, kit.Green
	case data.Launching:
		state, detail, spinner, color = locales.Launching(), data.Status, true, kit.Accent
	case data.Busy:
		state, detail, spinner = data.BusyStatus, locales.ReadyToLaunchWhenTheCurrentTaskFinishes(), true
		if state == "" {
			state = locales.WaitingForOtherTasks()
		}
	case data.Failure != "":
		state, detail, dot, color = locales.LaunchFailed(), data.Failure, true, kit.Red
	case data.OtherRunning != "":
		state, detail, dot, color = data.OtherRunning+" "+locales.Running3(), elapsed(data.OtherSince), true, kit.Green
	default:
		lastPlayed = detail != ""
	}
	status := ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center)
	type statusLayout struct{ viewport, width float32 }
	layout := ui.Local(status, "status-layout", func() statusLayout { return statusLayout{} })
	viewport, _ := c.Size()
	width := status.LayoutBounds().W
	if layout.viewport != viewport || layout.width != width {
		layout.viewport, layout.width = viewport, width
		// Check the committed width again after the footer has been laid out.
		c.After(0)
	}
	showState := !lastPlayed || width >= 16+8+kit.TextWidth(c, state, kit.Strong)+8+kit.TextWidth(c, detail, kit.Caption)
	status.Children(func() {
		ui.Box(c).Size(16, 16).Shrink(0).Center().Children(func() {
			if spinner {
				c.Key("status-spinner")
				ui.Spinner(c).Size(16, 16).TextColor(color)
			} else if dot {
				kit.StatusDot(c, color)
			} else {
				ui.Icon(c, icon).FontSize(16).TextColor(color)
			}
		})
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			if showState {
				c.Key("state")
				kit.Line(c, state, kit.Strong).Shrink(1).ContentTransition(kit.ValueMotion)
			}
			if detail != "" {
				c.Key("detail")
				kit.Line(c, detail, kit.Caption).Shrink(1).ContentTransition(kit.ValueMotion)
			}
			if data.Launching && !data.Running && !data.Finishing && !data.Canceling {
				kit.Progress(c, data.Progress).Absolute().Left(0).Bottom(-8).Width(kit.W(240)).MaxWidthPercent(100).Height(4)
			}
		})
		if data.Failure != "" && !data.Launching && !data.Busy {
			if data.FailureAction != "" && actions.Recover != nil {
				label := locales.Repair()
				if data.FailureAction == "login" {
					label = locales.SignInAgain()
				}
				kit.Ghost(c, label, actions.Recover).Height(kit.TabH())
			} else {
				kit.Ghost(c, locales.ViewLogs(), actions.ViewLog).Height(kit.TabH())
			}
		}
	})
}

func characterTile(c *ui.Context, data HomeData, actions HomeActions) {
	w, _ := c.Size()
	width := kit.W(176)
	compact := w < kit.W(176)+kit.W(160)+2*kit.Pad(c)+240
	if compact {
		width = 40
	}
	el := kit.RowButton(c, "", false, nil).Width(width).Height(kit.H(40, kit.Strong, kit.Caption)).Shrink(0).Radius(kit.RadiusM).Padding(0, 8).Gap(8).Border(1, kit.LineColor).Label(locales.ChooseAccount())
	if compact {
		el.Padding(0).Center().Tooltip(data.Account.Name)
	}
	el.Children(func() {
		if data.Account.ID == "" {
			ui.Box(c).Size(28, 28).Shrink(0).Radius(kit.RadiusS).Center().Background(kit.AccentSoft).Children(func() { ui.Icon(c, kit.User).FontSize(16).TextColor(kit.AccentText) })
		} else {
			kit.PlayerAvatar(c, data.Avatar, data.Account.Initial(), 28)
		}
		if compact {
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			name := data.Account.Name
			if name == "" {
				name = locales.AddAccount()
			}
			kit.Line(c, name, kit.Strong).ContentTransition(kit.ValueMotion)
			if data.Account.ID != "" {
				kit.Line(c, data.Account.Label(), kit.Caption)
			}
		})
		ui.Icon(c, kit.UpDown).FontSize(16).TextColor(kit.Muted)
	})
	kit.AccountMenu(c, el, data.Accounts, actions.SelectAccount, actions.AddAccount, actions.ManageAccounts)
}

type HomeRecent struct {
	Instance                    minecraft.Instance
	Name, Detail, Path, Address string
	Played                      time.Time
}

type HomeStatistics struct {
	Days                                 [7]float64
	Launches                             int
	LastPlayed                           time.Time
	Running                              string
	CPU                                  float64
	CPUReady                             bool
	MemoryUsed, MemoryTotal, LauncherMiB int
}

func (p *HomeState) recent(c *ui.Context, data HomeData, actions HomeActions) {
	kit.SectionHeader(c, locales.RecentlyPlayed(), func() { kit.Ghost(c, locales.All2(), actions.OpenLibrary).Height(kit.TabH()) })
	if len(data.Recent) == 0 {
		if data.Loading {
			kit.Loading(c)
		} else {
			kit.EmptyState(c, kit.Clock, locales.NoPlayHistory(), "", func() { kit.Button(c, locales.OpenLibrary(), actions.OpenLibrary) })
		}
		return
	}
	// Keep the list height stable while the window is resized; the page scroll
	// carries any rows that do not fit in the current viewport.
	count := min(6, len(data.Recent))
	ui.Column(c).FillWidth().Shrink(0).Margin(4, -8, 0, -8).Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			for _, recent := range data.Recent[:min(count, len(data.Recent))] {
				c.Key(recent.Instance.Key() + recent.Path + recent.Address)
				row := kit.RowButton(c, recent.Name, false, func() {
					if actions.OpenRecent != nil {
						actions.OpenRecent(recent)
					}
				}).Height(kit.RowDoubleH()).Padding(0, 8).Gap(12).Radius(kit.RadiusM)
				row.Children(func() {
					icon := kit.Folder
					if recent.Address != "" {
						icon = kit.Grid
					}
					kit.IconBadge(c, icon, 32)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						kit.Line(c, recent.Name, kit.Strong)
						ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
							kit.InstanceIcon(c, data.Icons[recent.Instance.Key()], 12)
							kit.Line(c, kit.MetaLine(recent.Instance.Name, recent.Detail), kit.Caption)
						})
					})
					ui.Box(c).Width(kit.W(72)).Shrink(0).AlignItems(ui.End).Children(func() {
						if (row.Hovered() || row.FocusVisible()) && !data.Launching && !data.Running {
							kit.QuickAction(c, false, locales.LaunchGame(), func() {
								if actions.LaunchRecent != nil {
									actions.LaunchRecent(recent)
								}
							}).Disabled(data.Busy)
						} else {
							kit.Line(c, relativePlayTime(recent.Played), kit.Caption).TextAlign(ui.End)
						}
					})
				})
			}
		})
	})
}

func homeStatistics(c *ui.Context, data HomeData) {
	s := data.Statistics
	kit.SectionHeader(c, locales.Last7Days(), func() {
		kit.Label(c, fmt.Sprintf(locales.Launches(), s.Launches), kit.Caption).SingleLine().Shrink(0)
	})
	total, peak := float64(0), float64(0)
	for _, v := range s.Days {
		total += v
		peak = max(peak, v)
	}
	ui.Row(c).Gap(8).AlignItems(ui.Baseline).Margin(4, 0, 16, 0).Children(func() {
		kit.Label(c, fmt.Sprintf("%.1f", total/3600), kit.Heading).SingleLine().Shrink(0).ContentTransition(kit.NumberMotion)
		kit.Label(c, locales.Hours(), kit.Caption).SingleLine().Shrink(0)
	})
	_, availableHeight := c.Size()
	height := 48 + (availableHeight-480)*0.1
	height = max(48, min(72, height))
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		for i, value := range s.Days {
			c.Key(i)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
				kit.ChartBar(c, value, peak, height, i == 6)
				day := time.Now().AddDate(0, 0, i-6).Weekday()
				label := []func() string{locales.Sun, locales.Mon, locales.Tue, locales.Wed, locales.Thu, locales.Fri, locales.Sat}[day]
				color := kit.Muted
				if i == 6 {
					label, color = locales.TodayCompact, kit.AccentText
				}
				kit.Label(c, label(), kit.Caption).SingleLine().Shrink(0).TextColor(color)
			})
		}
	})
}

func homeUsage(c *ui.Context, s HomeUsage) {
	kit.SectionHeader(c, locales.ResourceUsage(), func() { kit.MeterLegend(c, locales.Game(), locales.Other()) })
	cpuValue, memValue := "—", "—"
	cpuTip, memTip := "", ""
	cpuGame, cpuOther, memGame, memOther := float64(0), float64(0), float64(0), float64(0)
	if s.Ready {
		cpuValue = fmt.Sprintf("%.0f / %.0f%%", s.CPUGame, s.CPUTotal)
		cpuGame, cpuOther = s.CPUGame/100, max(0, s.CPUTotal-s.CPUGame)/100
		cpuTip = fmt.Sprintf(locales.GameUsed(), s.CPUGame, s.CPUTotal)
	}
	if s.MemTotalMiB > 0 {
		memValue = fmt.Sprintf("%.1f / %.1f / %.0f GB", float64(s.MemGameMiB)/1024, float64(s.MemUsedMiB)/1024, float64(s.MemTotalMiB)/1024)
		memGame = float64(s.MemGameMiB) / float64(s.MemTotalMiB)
		memOther = float64(max(0, s.MemUsedMiB-s.MemGameMiB)) / float64(s.MemTotalMiB)
		memTip = fmt.Sprintf(locales.GameGbUsedGbTotalGb(), float64(s.MemGameMiB)/1024, float64(s.MemUsedMiB)/1024, float64(s.MemTotalMiB)/1024)
	}
	totalGB := float64(max(0, s.MemTotalMiB)) / 1024
	valueWidth := max(kit.W(80), kit.TextWidth(c, "100 / 100%", kit.Caption), kit.TextWidth(c, fmt.Sprintf("%.1f / %.1f / %.0f GB", totalGB, totalGB, totalGB), kit.Caption))
	homeUsageMeter(c, "CPU", cpuGame, cpuOther, cpuValue, cpuTip, valueWidth)
	homeUsageMeter(c, locales.MemoryLabel(), memGame, memOther, memValue, memTip, valueWidth)
	launcherCPU, launcherMemory := "CPU —", "—"
	if s.LauncherMiB > 0 {
		launcherCPU = fmt.Sprintf("CPU %.1f%%", max(0, s.CPULauncher))
		launcherMemory = fmt.Sprintf("%d MB", s.LauncherMiB)
	}
	launcherLabel := locales.Launcher()
	launcherLabelWidth := max(kit.W(40), kit.TextWidth(c, launcherLabel, kit.Caption)+kit.W(4))
	ui.Row(c).FillWidth().Height(kit.H(28, kit.Caption)).Margin(4, 0, 0, 0).Gap(8).AlignItems(ui.Center).Children(func() {
		kit.Line(c, launcherLabel, kit.Caption).Width(launcherLabelWidth).Shrink(0)
		kit.Label(c, launcherCPU, kit.Caption).Grow(1).MinWidth(16).SingleLine().TextColor(kit.Text).ContentTransition(kit.NumberMotion)
		kit.Label(c, launcherMemory, kit.Caption).Width(kit.W(80)).Shrink(0).SingleLine().TextAlign(ui.End).TextColor(kit.Text).ContentTransition(kit.NumberMotion)
	})
}

func homeUsageMeter(c *ui.Context, label string, game, other float64, value, tooltip string, valueWidth float32) {
	ui.Column(c).FillWidth().Gap(4).Tooltip(tooltip).Children(func() {
		ui.Row(c).FillWidth().Height(kit.H(18, kit.Caption)).AlignItems(ui.Center).Children(func() {
			kit.Label(c, label, kit.Caption).Grow(1).MinWidth(0).SingleLine()
			kit.Label(c, value, kit.Caption).TextColor(kit.Text).Width(valueWidth).Shrink(0).SingleLine().TextAlign(ui.End).ContentTransition(kit.NumberMotion)
		})
		kit.MeterBar(c, game, other)
	})
}

func elapsed(at time.Time) string {
	if at.IsZero() {
		return "00:00:00"
	}
	seconds := max(0, int(time.Since(at).Seconds()))
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds/60%60, seconds%60)
}

func relativePlayTime(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	now := time.Now()
	at = at.Local()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if !at.Before(today) {
		return locales.Today() + " " + at.Format("15:04")
	}
	if !at.Before(today.AddDate(0, 0, -1)) {
		return locales.Yesterday() + " " + at.Format("15:04")
	}
	if at.Year() != now.Year() {
		return at.Format("2006-01-02")
	}
	return at.Format("01-02 15:04")
}
