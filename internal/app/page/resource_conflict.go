package page

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (p *Resources) requestInstall(version resourcecatalog.Version, data *ResourcesData, actions ResourcesActions) {
	if data.FullPage {
		// The downloads page browses without a target: pick a compatible
		// instance after choosing the version.
		p.pendingTarget = &version
		p.pendingTargetPolicy = resource.InstallReplace
		if p.conflictChoice == 1 {
			p.pendingTargetPolicy = resource.InstallKeepDisabled
		}
		p.targetOpen = true
		return
	}
	conflicts := resourceConflicts(data.Entries, version)
	if len(conflicts) == 0 {
		// Manually imported mods do not have an origin record yet. The
		// resolver can still identify them by their provider fingerprint, so
		// let it replace an existing version instead of surfacing a late
		// "enable or remove it first" error.
		policy := resource.InstallDefault
		if data.Kind == resource.Mod {
			policy = resource.InstallReplace
		}
		actions.Install(version, policy, "")
		return
	}
	p.pendingInstall = &version
	p.conflictEntries = conflicts
	p.conflictChoice = 0
	p.conflict = true
}

func resourceConflicts(entries []resource.Entry, version resourcecatalog.Version) []resource.Entry {
	var conflicts []resource.Entry
	for _, entry := range entries {
		for _, origin := range entry.Origins {
			if origin.Source == string(version.Source) && origin.ProjectID == version.ProjectID {
				conflicts = append(conflicts, entry)
				goto next
			}
		}
		// Origin metadata may be absent for files imported by another launcher.
		// Match the stable filename stem in that case, while avoiding a broad
		// substring match that would flag unrelated resources.
		if resourceNameStem(entry.Name) != "" && resourceNameStem(entry.Name) == resourceNameStem(version.Name) {
			conflicts = append(conflicts, entry)
		}
	next:
	}
	return conflicts
}

func resourceNameStem(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var out []rune
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= '0' && r <= '9':
			return string(out)
		}
	}
	return string(out)
}

func (p *Resources) targetDialog(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if p.pendingTarget == nil {
		p.targetOpen = false
		return
	}
	version := *p.pendingTarget
	targets := matchingResourceTargets(data, version, p.versionCompatibility)
	kit.Dialog(c, &p.targetOpen, kit.DialogM, func(_, panel ui.Element) {
		panel.MaxHeightPercent(85).Children(func() {
			kit.DialogHeader(c, locales.ChooseTargetInstance(), func() {
				p.targetOpen = false
				p.pendingTarget = nil
			})
			kit.Label(c, version.Name, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			if len(targets) == 0 {
				kit.EmptyState(c, kit.Cube, locales.NoCompatibleInstances(), locales.NoCompatibleInstancesHint(), nil)
				return
			}
			ui.Scroll(c).Scrollbars(false).FillWidth().Grow(1).MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Gap(4).Children(func() {
					for _, target := range targets {
						target := target
						row := kit.RowButton(c, "", false, func() {
							p.targetOpen = false
							p.pendingTarget = nil
							actions.Install(version, p.pendingTargetPolicy, target.Key())
						}).Padding(10, 12).Gap(10)
						row.Children(func() {
							kit.InstanceIcon(c, data.Icons[target.Key()], 32)
							ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
								kit.Label(c, target.Name, kit.Strong).MaxLines(1).Ellipsis("…")
								kit.Label(c, kit.MetaLine(target.GameVersion, target.Loader), kit.Caption).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
							})
						})
					}
				})
			})
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.Cancel(), func() {
					p.targetOpen = false
					p.pendingTarget = nil
				})
			})
		})
	})
}

// matchingResourceTargets keeps the instances a chosen version can actually be
// installed into under the filters the user picked. An explicit game version
// or loader narrows the list; "all" falls back to what the version declares.
// A version that lists no constraint matches every instance of that kind.
func matchingResourceTargets(data *ResourcesData, version resourcecatalog.Version, compatibility resourcecatalog.CompatibilityFilter) []minecraft.Instance {
	filter := compatibility.Apply(resourcecatalog.Filter{GameVersion: data.GameVersion, Loaders: []string{loaderID(data.Loader)}})
	game, loader := filter.GameVersion, ""
	if data.Kind == resource.Mod {
		loader = strings.Join(filter.Loaders, " / ")
	}
	result := make([]minecraft.Instance, 0, len(data.Targets))
	for _, instance := range data.Targets {
		if !instance.CanLaunch() {
			continue
		}
		if game != "" && instance.GameVersion != game {
			continue
		}
		if game == "" && len(version.GameVersions) > 0 && !slices.Contains(version.GameVersions, instance.GameVersion) {
			continue
		}
		if data.Kind == resource.Mod && len(version.Loaders) > 0 {
			required := version.Loaders
			if loader != "" {
				required = []string{loader}
			}
			instanceLoader := loaderID(instance.Loader)
			matched := slices.ContainsFunc(instance.Loaders, func(value string) bool {
				return slices.ContainsFunc(required, func(item string) bool { return strings.EqualFold(value, item) })
			}) || slices.ContainsFunc(required, func(item string) bool { return strings.EqualFold(instanceLoader, item) })
			if !matched {
				continue
			}
		}
		result = append(result, instance)
	}
	return result
}

func (p *Resources) conflictDialog(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	kit.Dialog(c, &p.conflict, kit.DialogS, func(_, panel ui.Element) {
		panel.MaxHeightPercent(85)
		panel.Children(func() {
			kit.DialogHeader(c, locales.HandleDuplicateResource(), func() {
				p.conflict = false
				p.pendingInstall = nil
				p.conflictEntries = nil
			})
			if p.pendingInstall == nil {
				return
			}
			version := *p.pendingInstall
			kit.Label(c, version.Name+" · "+version.Number, kit.Body).TextColor(kit.Muted).MaxLines(1).Ellipsis("…")
			ui.Column(c).FillWidth().Gap(8).Padding(12).Radius(kit.RadiusM).Background(kit.InsetColor).Children(func() {
				kit.Label(c, locales.TargetInstance()+"："+data.Instance, kit.Body).MaxLines(1).Ellipsis("…")
				kit.Label(c, locales.AnExistingResourceFromThisProjectWasFound(), kit.Body).TextColor(kit.Muted)
				for _, entry := range p.conflictEntries {
					kit.Label(c, entry.File, kit.Caption).MaxLines(1).Ellipsis("…")
				}
			})
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				for _, option := range []struct {
					value int
					title string
					hint  string
				}{
					{0, locales.ReplaceExistingVersionRecommended(), locales.DownloadTheNewVersionAndRemoveTheOldFile()},
					{1, locales.DisableAndKeepExistingVersion(), locales.KeepTheOldFileAndDisableIt()},
				} {
					selected := p.conflictChoice == option.value
					row := kit.RowButton(c, "", selected, func() { p.conflictChoice = option.value }).Padding(12, 12).Gap(8)
					row.Children(func() {
						ui.Radio(c, &p.conflictChoice, option.value, option.title)
						kit.Label(c, option.hint, kit.Caption).Grow(1).MinWidth(0).MaxLines(2).Ellipsis("…")
					})
				}
			})
			kit.DialogFooter(c, func() {
				kit.Button(c, locales.Cancel(), func() {
					p.conflict = false
					p.pendingInstall = nil
					p.conflictEntries = nil
				})
				label := locales.DownloadAndReplace()
				if p.conflictChoice == 1 {
					label = locales.DownloadAndDisableExisting()
				}
				kit.Primary(c, label, func() {
					policy := resource.InstallReplace
					if p.conflictChoice == 1 {
						policy = resource.InstallKeepDisabled
					}
					p.conflict = false
					p.pendingInstall = nil
					p.conflictEntries = nil
					actions.Install(version, policy, "")
				})
			})
		})
	})
}
