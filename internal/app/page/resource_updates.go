package page

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Resources) updates(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if !data.UpdatesOpen {
		p.updateSpaceItems, p.updateSpaceKey = nil, ""
		p.updateCheck, p.updateList, p.updateSelected = nil, ui.ListState{}, ui.Selection[string]{}
		return
	}
	if p.updateCheck != data.UpdateCheck {
		p.updateSpaceDirty = true
		p.updateCheck, p.updateList, p.updateSelected = data.UpdateCheck, ui.ListState{}, ui.Selection[string]{}
		if data.UpdateCheck != nil {
			for _, item := range data.UpdateCheck.Updates {
				if item.Version.Downloadable {
					p.updateSelected.Add(item.Entry.Relative)
				}
			}
		}
	}
	services := c.Services()
	width, _ := c.Size()
	height := float32(320)
	if data.UpdateCheck != nil {
		height = float32(min(510, max(320, 220+len(data.UpdateCheck.Updates)*68)))
	}
	if data.UpdateDetail != nil {
		height = 510
	}
	if data.MissingOpen {
		height = float32(min(510, max(360, 250+len(data.Missing)*58)))
	}
	kit.Dialog(c, &data.UpdatesOpen, kit.DialogL, func(back, panel ui.Element) {
		panel.Height(height).MaxHeightPercent(90)
		panel.Children(func() {
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				if data.MissingOpen {
					kit.Button(c, locales.Back(), actions.CloseMissing).Disabled(data.Busy || data.MissingBusy)
				}
				title := locales.ResourceUpdates()
				if data.MissingOpen {
					title = locales.AddMissingResourceFiles()
				}
				kit.Label(c, title, kit.Body).Grow(1)
				kit.IconButton(c, kit.Close, false, actions.CloseUpdates).Label(locales.CloseResourceUpdates())
			})
			if data.MissingOpen {
				p.missingBody(c, data, actions)
				return
			}
			if data.UpdateDetail != nil {
				item := data.UpdateDetail
				ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
					kit.Button(c, locales.Back(), actions.BackUpdates)
					kit.Label(c, item.Entry.DisplayName(), kit.Body).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
					kit.Button(c, locales.ViewWebsite(), func() { services.OpenURL(item.Version.Website) }).Disabled(item.Version.Website == "")
				})
				kit.Label(c, item.Origin.Number+" → "+item.Version.Number, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				if data.UpdateError != "" {
					kit.ErrorText(c, data.UpdateError)
				}
				if data.UpdatesLoading {
					ui.Column(c).Grow(1).FillWidth().Center().Children(func() { ui.Spinner(c) })
					return
				}
				if len(data.UpdateDetails.GameVersions) > 0 {
					kit.Label(c, locales.CompatibleVersions()+" · "+strings.Join(data.UpdateDetails.GameVersions, ", "), kit.Body).TextColor(kit.Muted).MaxLines(2).Ellipsis("…")
				}
				if len(data.UpdateDetails.Loaders) > 0 {
					kit.Label(c, strings.Join(data.UpdateDetails.Loaders, ", "), kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
				}
				ui.Scroll(c).Scrollbars(false).Grow(1).FillWidth().MinHeight(0).Children(func() {
					text := data.UpdateDetails.Changelog
					if text == "" {
						text = locales.NoChangelogAvailable()
					}
					kit.Label(c, text, kit.Body).FillWidth().Wrap()
				})
				return
			}
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				kit.Select(c, &data.UpdateSource, data.UpdateSource.Label(), []kit.SelectOption[resourcecatalog.Source]{{Value: resourcecatalog.Modrinth, Label: "Modrinth"}, {Value: resourcecatalog.CurseForge, Label: "CurseForge"}}, 130, 160, actions.CheckUpdates).Label(locales.UpdateSource()).Disabled(data.Busy)
				hint := "Minecraft " + data.GameVersion
				if data.Loader != "" {
					hint += " · " + data.Loader
				}
				kit.Label(c, hint, kit.Body).TextColor(kit.Muted).Grow(1).MinWidth(0).MaxLines(1).Ellipsis("…")
				c.Key("check-resource-updates")
				if width < 520 {
					kit.IconButton(c, kit.Refresh, false, actions.CheckUpdates).Label(locales.CheckForUpdates()).Tooltip(locales.CheckForUpdates()).Disabled(data.Busy || data.UpdatesLoading)
				} else {
					kit.Button(c, locales.CheckForUpdates(), actions.CheckUpdates).Disabled(data.Busy || data.UpdatesLoading)
				}
			})
			if data.UpdateError != "" {
				kit.ErrorText(c, data.UpdateError)
			}
			kit.Card(c, ui.Column(c)).Grow(1).MinHeight(0).Clip().Children(func() {
				if data.UpdatesLoading {
					kit.Loading(c)
					return
				}
				if data.UpdateCheck == nil || len(data.UpdateCheck.Updates) == 0 {
					label := locales.CheckForCompatibleReleases()
					if data.UpdateCheck != nil {
						label = locales.NoUpdatesAvailable()
					}
					ui.Column(c).Fill().Center().Children(func() { kit.Label(c, label, kit.Body).TextColor(kit.Muted) })
					return
				}
				c.Key("resource-updates")
				kit.PresenceMotion(ui.List(c, &p.updateList, len(data.UpdateCheck.Updates)).Grow(1).FillWidth().Padding(0, kit.ScrollbarGutter, 0, 0)).ItemKey(func(i int) any { return data.UpdateCheck.Updates[i].Entry.Relative }).Scrollbars(false).Rows(func(row ui.ListRow) {
					c := row.Context
					item := data.UpdateCheck.Updates[row.Index]
					ui.Row(c).FillWidth().Padding(12).Gap(8).AlignItems(ui.Center).Children(func() {
						selected := p.updateSelected.Has(item.Entry.Relative)
						ui.Checkbox(c, &selected, "").Label(locales.SelectUpdate() + item.Entry.File).Disabled(data.Busy).OnClick(func() {
							p.updateSpaceDirty = true
							if p.updateSelected.Has(item.Entry.Relative) {
								p.updateSelected.Remove(item.Entry.Relative)
							} else {
								p.updateSelected.Add(item.Entry.Relative)
							}
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
							kit.Label(c, item.Entry.DisplayName(), kit.Body).MaxLines(1).Ellipsis("…")
							kit.Label(c, item.Origin.Number+" → "+item.Version.Number, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
							if !item.Version.Downloadable {
								kit.Label(c, locales.ManualDownloadRequired(), kit.Body).TextColor(kit.Muted)
							} else if !item.Entry.Enabled {
								kit.Label(c, locales.KeepDisabled(), kit.Body).TextColor(kit.Muted)
							}
						})
						kit.Button(c, locales.Details(), func() { actions.UpdateDetails(item) }).Label(locales.ViewUpdateDetails() + item.Entry.File).Disabled(data.Busy)
					})
					if row.Index < len(data.UpdateCheck.Updates)-1 {
						kit.Divider(c).Margin(0, 12)
					}
				})
			})
			if data.UpdateCheck != nil && (data.UpdateCheck.Unknown > 0 || data.UpdateCheck.Conflicts > 0) {
				kit.Label(c, fmt.Sprintf(locales.SkippedUnrecognizedDuplicates(), data.UpdateCheck.Unknown, data.UpdateCheck.Conflicts), kit.Body).TextColor(kit.Muted).MaxLines(2).Ellipsis("…")
			}
			space := SpaceData{}
			if actions.AssessUpdates != nil && data.UpdateCheck != nil {
				if p.updateSpaceDirty {
					p.updateSpaceDirty = false
					p.updateSpaceItems = p.updateSpaceItems[:0]
					for _, item := range data.UpdateCheck.Updates {
						if p.updateSelected.Has(item.Entry.Relative) {
							p.updateSpaceItems = append(p.updateSpaceItems, item)
						}
					}
					p.updateSpaceVersion++
					p.updateSpaceKey = fmt.Sprintf("%d:%p:%d", data.ID, p.updateCheck, p.updateSpaceVersion)
				}
				space = actions.AssessUpdates(p.updateSpaceKey, p.updateSpaceItems)
				SpaceSummary(c, space)
			}
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				kit.Label(c, fmt.Sprintf(locales.Selected(), p.updateSelected.Len()), kit.Body).TextColor(kit.Muted).Grow(1)
				kit.Primary(c, locales.UpdateSelected(), func() {
					var chosen []resourcecatalog.ResourceUpdate
					if data.UpdateCheck != nil {
						for _, item := range data.UpdateCheck.Updates {
							if p.updateSelected.Has(item.Entry.Relative) {
								chosen = append(chosen, item)
							}
						}
					}
					actions.Update(chosen)
				}).Disabled(data.Busy || data.UpdatesLoading || p.updateSelected.Len() == 0 || space.Blocked())
			})
		})
	})
	if !data.UpdatesOpen {
		actions.CloseUpdates()
	}
}
