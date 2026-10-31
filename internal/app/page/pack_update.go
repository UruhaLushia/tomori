package page

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

type PackUpdateData struct {
	Open, Loading, Busy, HasOrigin                            bool
	Key, Name, Error, Path, URL, Status, Backup               string
	Mode                                                      int
	Origin                                                    modpack.OriginHeader
	Versions                                                  []packcatalog.Version
	Selected                                                  string
	Info                                                      modpack.Info
	Optional, Deferred, DifferentSource, AllowDifferentSource bool
	OptionalAvailable                                         bool
	Plan                                                      *modpack.UpdatePlan
	Conflicts                                                 []int
	ShowConflicts                                             bool
	Counts                                                    [3]int
	Choices                                                   map[string]modpack.UpdateChoice
	Missing                                                   []modpack.MissingFile
	LocalCount                                                int
	Space                                                     SpaceData
}

type PackUpdateActions struct {
	Close, Browse, FetchURL, Prepare, Apply, Back, ChoicesChanged, SourceChanged, Supplement, OpenBackup func()
	Choose                                                                                               func(packcatalog.Version)
	Drop                                                                                                 func([]string)
}

type PackUpdate struct {
	versions, files, missing ui.ListState
	plan                     *modpack.UpdatePlan
}

func (p *PackUpdate) View(c *ui.Context, data *PackUpdateData, actions PackUpdateActions) {
	if !data.Open {
		return
	}
	if p.plan != data.Plan {
		p.plan, p.files = data.Plan, ui.ListState{}
	}
	c.Key("pack-update:" + data.Key)
	dialog := kit.Dialog(c, &data.Open, kit.DialogL, func(back, panel ui.Element) {
		panel.Height(620)
		if data.Backup != "" {
			panel.Height(280)
		} else if data.Plan == nil && data.Mode != 0 && len(data.Missing) == 0 {
			panel.Height(440)
		}
		if actions.Drop != nil && !data.Busy && !data.Loading && data.Plan == nil && data.Backup == "" {
			if panel.FileDragOver() {
				panel.Border(1, kit.Accent)
			}
			if paths := panel.DroppedFiles(); len(paths) > 0 {
				actions.Drop(paths)
			}
		}
		panel.Children(func() {
			kit.DialogHeader(c, locales.UpdateModpack(), actions.Close).Label(locales.CloseModpackUpdate())
			kit.Label(c, data.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if data.Error != "" {
				kit.ErrorText(c, data.Error).MaxLines(3).Ellipsis("…")
			}
			if data.Backup != "" {
				ui.Column(c).Grow(1).FillWidth().Justify(ui.Center).Gap(8).Children(func() {
					kit.Label(c, locales.UpdateComplete(), kit.Body)
					kit.Label(c, data.Backup, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				})
				kit.DialogFooter(c, func() { kit.Button(c, locales.OpenBackupFolder(), actions.OpenBackup) })
				return
			}
			if data.Busy {
				ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Children(func() {
					ui.Spinner(c)
					kit.Label(c, data.Status, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				})
				return
			}
			if data.Plan != nil {
				p.review(c, data, actions)
				return
			}
			if !data.HasOrigin {
				ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
					if data.Loading {
						ui.Spinner(c)
					}
				})
				return
			}
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				kit.Label(c, locales.CurrentVersion(), kit.Body).TextColor(kit.Muted)
				kit.Label(c, packVersion(data.Origin.Pack.Version), kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			})
			if len(data.Missing) > 0 {
				p.supplement(c, data, actions)
				return
			}
			kit.Segmented(c, &data.Mode, locales.OnlineVersions(), locales.LocalFile(), "URL").FillWidth().OnChange(actions.SourceChanged)
			switch data.Mode {
			case 0:
				p.versionList(c, data, actions)
			case 1:
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					label := locales.ZipMrpackDropAFile()
					if data.Path != "" {
						label = filepath.Base(data.Path)
					}
					kit.Label(c, label, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					kit.Button(c, locales.ChooseFiles(), actions.Browse).Disabled(data.Loading)
				})
				ui.Spacer(c)
			default:
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					kit.Input(c, &data.URL).Grow(1).MinWidth(0).Placeholder(locales.DownloadLinkOrProjectPage()).Label(locales.ModpackUrl()).OnSubmit(actions.FetchURL).Disabled(data.Loading)
					kit.Button(c, locales.ReadingModpack(), actions.FetchURL).Disabled(data.Loading || strings.TrimSpace(data.URL) == "")
				})
				ui.Spacer(c)
			}
			if data.Loading {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Spinner(c).Size(16, 16)
					kit.Label(c, locales.Loading(), kit.Body).TextColor(kit.Muted)
				})
			}
			if data.Info.Format != "" {
				label := packVersion(data.Info.Version) + " · Minecraft " + packVersion(data.Info.GameVersion)
				if loader := data.Info.LoaderLabel(); loader != "" {
					label += " · " + loader
				}
				kit.Label(c, label, kit.Body).TextColor(kit.Muted).MaxLines(2).Wrap()
				if data.OptionalAvailable {
					ui.Checkbox(c, &data.Optional, locales.InstallOptionalFiles2())
				}
				if data.DifferentSource {
					ui.Checkbox(c, &data.AllowDifferentSource, locales.ChangeModpackSource())
				}
				if len(data.Info.Warnings) > 0 {
					kit.ErrorText(c, strings.Join(data.Info.Warnings, "；"))
				}
			}
			SpaceSummary(c, data.Space)
			kit.DialogFooter(c, func() {
				kit.Primary(c, locales.CheckingFiles(), actions.Prepare).Disabled(data.Loading || data.Info.Format == "" || len(data.Info.Warnings) > 0 || data.DifferentSource && !data.AllowDifferentSource || data.Space.Blocked())
			})
		})
	})
	if dialog.Dismissed() {
		actions.Close()
	}
}

func packVersion(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func (p *PackUpdate) versionList(c *ui.Context, data *PackUpdateData, actions PackUpdateActions) {
	ui.List(c, &p.versions, len(data.Versions)).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).
		ItemKey(func(i int) any { return data.Versions[i].ID }).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		v := data.Versions[row.Index]
		button := kit.RowButton(c, "", false, func() { actions.Choose(v) }).FillWidth().Padding(12, 12).Gap(8).Disabled(v.URL == "")
		if data.Selected == v.ID {
			button.Background(kit.AccentSoft)
		} else if button.Hovered() {
			button.Background(kit.SurfaceHi)
		}
		button.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				kit.Label(c, v.Name, kit.Body).MaxLines(1).Ellipsis("…")
				kit.Label(c, v.Number+" · "+v.Published.Format("2006-01-02"), kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			})
			if v.ID == data.Origin.Source.VersionID {
				kit.Pill(c, locales.Current(), kit.AccentTone)
			}
		})
	})
	if !data.Loading && len(data.Versions) == 0 {
		kit.Label(c, locales.NoMatchingVersions(), kit.Body).TextColor(kit.Muted)
	}
}

func (p *PackUpdate) review(c *ui.Context, data *PackUpdateData, actions PackUpdateActions) {
	kit.Label(c, packVersion(data.Origin.Pack.Version)+" → "+packVersion(data.Info.Version), kit.Body).MaxLines(1).Ellipsis("…")
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		kit.Label(c, fmt.Sprintf(locales.AddedReplacedRemoved(), data.Counts[0], data.Counts[1], data.Counts[2]), kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).Wrap()
		if len(data.Conflicts) > 0 {
			ui.Checkbox(c, &data.ShowConflicts, fmt.Sprintf(locales.Conflicts(), len(data.Conflicts))).OnChange(func() { p.files = ui.ListState{} })
		}
	})
	count := data.Plan.Len()
	if data.ShowConflicts {
		count = len(data.Conflicts)
	}
	c.Key(data.Plan)
	ui.List(c, &p.files, count).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any {
		if data.ShowConflicts {
			i = data.Conflicts[i]
		}
		return data.Plan.Entry(i).Key
	}).Scrollbars(false).Rows(func(row ui.ListRow) {
		index := row.Index
		if data.ShowConflicts {
			index = data.Conflicts[index]
		}
		entry := data.Plan.Entry(index)
		c := row.Context
		holder := ui.Row(c).FillWidth().Padding(8, 4).Gap(12).AlignItems(ui.Center)
		holder.Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				kit.Label(c, entry.Path, kit.Body).MaxLines(1).Ellipsis("…")
				label := updateReason(entry.Reason)
				if entry.Disabled {
					label += " · " + locales.KeepDisabled()
				}
				kit.Label(c, label, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			})
			choice := entry.Default
			if value, ok := data.Choices[entry.Key]; ok {
				choice = value
			}
			if entry.Reason == modpack.UpdateUnsafe || entry.Reason == modpack.UpdateAlreadyCurrent {
				kit.Label(c, locales.Keep(), kit.Body).TextColor(kit.Muted)
			} else {
				value := ui.Local(holder, "choice", func() modpack.UpdateChoice { return choice })
				label := locales.Keep()
				use := locales.UseNewFile()
				if entry.Kind == modpack.UpdateRemove {
					use = locales.Delete()
				}
				if *value == modpack.UpdateUsePack {
					label = use
				}
				kit.Select(c, value, label, []kit.SelectOption[modpack.UpdateChoice]{{Value: modpack.UpdateKeep, Label: locales.Keep()}, {Value: modpack.UpdateUsePack, Label: use}}, 110, 130, func() {
					if data.Choices == nil {
						data.Choices = make(map[string]modpack.UpdateChoice)
					}
					data.Choices[entry.Key] = *value
					actions.ChoicesChanged()
				}).Label(locales.FileAction() + " " + entry.Path)
			}
		})
	})
	SpaceSummary(c, data.Space)
	kit.DialogFooter(c, func() {
		kit.Button(c, locales.Back(), actions.Back)
		kit.Primary(c, locales.Update(), actions.Apply).Disabled(data.Space.Blocked())
	})
}

func updateReason(reason modpack.UpdateReason) string {
	switch reason {
	case modpack.UpdateModified:
		return locales.Modified()
	case modpack.UpdateUserAdded:
		return locales.UserAdded()
	case modpack.UpdateUserDeleted:
		return locales.UserDeleted()
	case modpack.UpdatePersonal:
		return locales.PersonalData()
	case modpack.UpdateUnsafe:
		return locales.PathConflict()
	case modpack.UpdateAlreadyCurrent:
		return locales.AlreadyUpToDate()
	case modpack.UpdateDisabledChange:
		return locales.DisabledResourceUpdated()
	default:
		return locales.Unmodified()
	}
}

func (p *PackUpdate) supplement(c *ui.Context, data *PackUpdateData, actions PackUpdateActions) {
	services := c.Services()
	ui.List(c, &p.missing, len(data.Missing)).Grow(1).FillWidth().MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any { return data.Missing[i].Path }).Scrollbars(false).Rows(func(row ui.ListRow) {
		file := data.Missing[row.Index]
		c := row.Context
		ui.Row(c).FillWidth().Padding(8, 0).Gap(8).AlignItems(ui.Center).Children(func() {
			kit.Label(c, file.Name, kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
			kit.Button(c, locales.DownloadInBrowser(), func() { services.OpenURL(file.Website) }).Disabled(file.Website == "")
		})
	})
	if data.LocalCount > 0 {
		kit.Label(c, fmt.Sprintf(locales.AddedFiles(), data.LocalCount), kit.Body).TextColor(kit.Muted)
	}
	kit.DialogFooter(c, func() {
		kit.Button(c, locales.ChooseFiles(), actions.Supplement)
		kit.Primary(c, locales.Retry(), actions.Prepare)
	})
}
