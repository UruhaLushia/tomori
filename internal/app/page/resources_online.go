package page

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
	"github.com/egoist/mygo/i18n"
)

func (p *Resources) online(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if data.Project != nil {
		p.versions(c, data, actions)
		return
	}
	p.onlineFullPage(c, data, actions)
}

func (p *Resources) onlineFullPage(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	kit.Toolbar(c, func() {
		sources := []kit.SelectOption[resourcecatalog.Source]{{Value: resourcecatalog.CurseForge, Label: "CurseForge"}}
		if data.Kind != resource.World {
			sources = append(sources, kit.SelectOption[resourcecatalog.Source]{Value: resourcecatalog.Modrinth, Label: "Modrinth"})
		}
		if data.Source != resourcecatalog.CurseForge && data.Kind == resource.World {
			data.Source = resourcecatalog.CurseForge
		}
		kit.FilterSelect(c, &data.Source, locales.Source3()+" · "+data.Source.Label(), sources, 170, func() { actions.SourceChanged(p.query) }).Label(locales.Source())
		p.categoryFilter(c, data, actions)
		p.compatibilityFilter(c, data, actions)
		ui.Spacer(c)
		kit.SearchField(c, &p.query, locales.SearchResources(), data.Loading).OnSubmit(func() { actions.Search(p.query, 0) })
	})
	if data.Search.ResolvedQuery != "" {
		kit.Line(c, fmt.Sprintf(locales.SearchingByOriginalName(), data.Search.ResolvedQuery), kit.Caption)
	} else if data.Search.NameWarning {
		kit.Line(c, locales.ChineseNameTranslationsUnavailable(), kit.Caption)
	}
	if len(data.Search.Suggestions) > 0 {
		options := make([]kit.SelectOption[string], 0, len(data.Search.Suggestions))
		for _, suggestion := range data.Search.Suggestions {
			options = append(options, kit.SelectOption[string]{Value: suggestion.Query, Label: suggestion.Name + " · " + suggestion.Query})
		}
		kit.FilterSelect(c, &p.nameChoice, locales.ChooseAMatchingName(), options, 320, func() { p.query = p.nameChoice; actions.Search(p.query, 0) })
	}
	ui.Box(c).Height(4).Shrink(0)
	count := len(data.Search.Hits)
	if count == 0 {
		switch {
		case data.Loading:
			kit.Loading(c)
		case data.Error != "":
			kit.EmptyState(c, kit.Download, locales.CouldNotLoad(), data.Error, func() { kit.Ghost(c, locales.Retry(), func() { actions.Search(p.query, 0) }) })
		default:
			kit.EmptyState(c, kit.Cube, locales.NoMatchingResources(), locales.TryAnotherSearchOrAdjustTheFilters(), nil)
		}
		return
	}
	more := count < data.Search.Total
	c.Key("online-results")
	visible := make(map[string]struct{})
	ui.List(c, &p.onlineList, count+1).Grow(1).FillWidth().MinHeight(0).Margin(0, -8).ItemKey(func(i int) any {
		if i == count {
			return "online-end"
		}
		return data.Search.Hits[i].ID
	}).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		if row.Index == count {
			kit.ListEnd(c, count, data.Loading, data.Error, more, func() { actions.Search(data.Query, data.SearchOffset) })
			return
		}
		project := data.Search.Hits[row.Index]
		visible[project.ID] = struct{}{}
		installed, number := installedProject(data.Entries, project)
		kit.ListRow(c, kit.Row{Title: project.DisplayName(), Subtitle: project.DisplayDescription(), Lead: func() {
			if actions.EnsureVideo != nil {
				actions.EnsureVideo(project.ID)
			}
			kit.ProjectIcon(c, data.Icons[project.ID], 32, data.Videos[project.ID])
		}, Badge: func() {
			if project.Author != "" {
				kit.Line(c, project.Author, kit.Caption).MinWidth(0).Shrink(100)
			}
			resourceProjectBadges(c, project)
			if installed {
				label := locales.Installed3()
				if number != "" {
					label = locales.Installed4() + number
				}
				kit.Badge(c, label, kit.AccentTone)
			}
		}, OnClick: func() { actions.Versions(project) }}, func(bool) {
			if !kit.Compact(c) {
				ui.Column(c).Width(kit.W(120)).Shrink(0).AlignItems(ui.End).Children(func() {
					kit.Line(c, downloadCount(project.Downloads)+" "+locales.Downloads6(), kit.Caption)
				})
			}
		})
	})
	if actions.ReleaseVideos != nil {
		actions.ReleaseVideos(visible)
	}
	if !data.Loading && data.Error == "" && more && kit.NearListEnd(c, &p.onlineList, count) {
		actions.Search(data.Query, data.Search.Offset+resourcecatalog.PageSize)
	}
}

func resourceProjectBadges(c *ui.Context, project resourcecatalog.Project) {
	nameLength := len([]rune(project.DisplayName()))
	visible := 1
	if nameLength > 24 {
		visible = 0
	}
	visible = min(visible, len(project.Categories))
	for _, category := range project.Categories[:visible] {
		kit.Badge(c, category, kit.Dim)
	}
	kit.Badge(c, project.Source.Label(), kit.Dim)
	if remaining := len(project.Categories) - visible; remaining > 0 {
		kit.Badge(c, fmt.Sprintf("+%d", remaining), kit.Dim)
	}
}

func (p *Resources) categoryFilter(c *ui.Context, data *ResourcesData, actions ResourcesActions) ui.Element {
	options := resourceCategoryOptions(data.Kind, data.Source)
	value := data.Category
	return kit.FilterSelect(c, &value, resourceCategoryLabel(data, options), options, 230, func() {
		if actions.CategoryChanged != nil {
			actions.CategoryChanged(value)
		}
	}).Disabled(len(options) == 1 || actions.CategoryChanged == nil).Label(locales.ResourceCategory())
}

func resourceCategoryLabel(data *ResourcesData, options []kit.SelectOption[string]) string {
	label := locales.All()
	for _, option := range options {
		if option.Value == data.Category {
			label = option.Label
			break
		}
	}
	return locales.Category() + " · " + label
}

func resourceCategoryOptions(kind resource.Kind, source resourcecatalog.Source) []kit.SelectOption[string] {
	options := []kit.SelectOption[string]{{Value: "", Label: locales.All()}}
	if source == resourcecatalog.CurseForge {
		return append(options, curseForgeCategoryOptions(kind)...)
	}
	var categories []kit.SelectOption[string]
	switch kind {
	case resource.Mod:
		categories = []kit.SelectOption[string]{{Value: "optimization", Label: locales.Optimization()}, {Value: "utility", Label: locales.Utility()}, {Value: "technology", Label: locales.Technology()}, {Value: "magic", Label: locales.Magic()}, {Value: "adventure", Label: locales.Adventure2()}, {Value: "decoration", Label: locales.Decoration()}}
	case resource.ResourcePack:
		categories = []kit.SelectOption[string]{{Value: "realistic", Label: locales.Realistic()}, {Value: "themed", Label: locales.Theme()}, {Value: "simplistic", Label: locales.Minimal()}, {Value: "vanilla-like", Label: locales.VanillaLike()}, {Value: "pvp", Label: "PvP"}}
	case resource.ShaderPack:
		categories = []kit.SelectOption[string]{{Value: "realistic", Label: locales.Realistic()}, {Value: "semi-realistic", Label: locales.SemiRealistic()}, {Value: "cartoon", Label: locales.Cartoon()}, {Value: "fantasy", Label: locales.Fantasy()}, {Value: "vanilla-like", Label: locales.VanillaLike()}}
	}
	return append(options, categories...)
}

func curseForgeCategoryOptions(kind resource.Kind) []kit.SelectOption[string] {
	category := func(id int, label string) kit.SelectOption[string] {
		return kit.SelectOption[string]{Value: fmt.Sprintf("cf:%d", id), Label: label}
	}
	switch kind {
	case resource.Mod:
		return []kit.SelectOption[string]{
			category(422, locales.Adventure2()), category(416, locales.Farming()), category(412, locales.Technology()),
			category(419, locales.Magic()), category(406, locales.Worlds()), category(435, locales.Utility()),
			category(421, locales.ApiAndLibrary()), category(6814, locales.Performance()),
		}
	case resource.ResourcePack:
		return []kit.SelectOption[string]{
			category(393, "16x"), category(394, "32x"), category(395, "64x"), category(396, "128x"),
			category(397, "256x"), category(398, "512x+"), category(400, locales.Realistic()), category(401, locales.Modern()),
		}
	case resource.ShaderPack:
		return []kit.SelectOption[string]{category(6553, locales.Realistic()), category(6554, locales.Fantasy()), category(6555, locales.VanillaLike())}
	case resource.World:
		return []kit.SelectOption[string]{
			category(248, locales.Adventure()), category(249, locales.Creative()), category(250, locales.GameMap()),
			category(4464, locales.ModdedWorld()), category(251, locales.Parkour()), category(252, locales.Puzzle()), category(253, locales.Survival()),
		}
	case resource.DataPack:
		return []kit.SelectOption[string]{
			category(6948, locales.Adventure()), category(6952, locales.Magic()), category(6949, locales.Fantasy()),
			category(6951, locales.Technology()), category(6953, locales.Utility()), category(6947, locales.Other()),
		}
	}
	return nil
}

func (p *Resources) versions(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	p.versionsFullPage(c, data, actions)
}

func (p *Resources) versionsFullPage(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	project := data.Project
	if project == nil {
		return
	}
	services := c.Services()
	ui.Column(c).FillWidth().Shrink(0).Gap(12).Children(func() {
		kit.BackBar(c, data.Kind.Label(), actions.Back, nil)
		kit.IdentityHeader(c, kit.Identity{Icon: data.Icons[project.ID], Video: data.Videos[project.ID], Name: project.DisplayName(), Meta: kit.MetaLine(project.Author, downloadCount(project.Downloads)+" "+locales.Downloads6())}, nil, func() {
			if project.Website != "" {
				kit.IconButton(c, kit.External, false, func() { services.OpenURL(project.Website) }).Label(locales.OpenProjectWebsite())
			}
		})
	})
	ui.Box(c).Height(8).Shrink(0)
	kit.Toolbar(c, func() {
		games := []kit.SelectOption[string]{{Value: resourcecatalog.AnyCompatibility, Label: locales.All()}}
		loaders := []kit.SelectOption[string]{{Value: resourcecatalog.AnyCompatibility, Label: locales.All()}}
		for _, v := range data.Versions {
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
		changed := func() { p.versionList = ui.ListState{} }
		kit.FilterSelect(c, &p.versionCompatibility.GameVersion, p.versionGameLabel(data), games, 200, changed).Label(locales.GameVersion())
		kit.FilterSelect(c, &p.versionCompatibility.Loader, p.versionLoaderLabel(data), loaders, 200, changed).Label(locales.Loader())
		kit.FilterSelect(c, &p.versionChannel, p.versionChannelLabel(), []kit.SelectOption[string]{{Value: "", Label: locales.All()}, {Value: "release", Label: locales.Release()}, {Value: "beta", Label: "Beta"}, {Value: "alpha", Label: "Alpha"}}, 180, changed).Label(locales.Channel())
		ui.Spacer(c)
		if installed, number := installedProject(data.Entries, *project); installed {
			if !kit.Compact(c) {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() { kit.StatusDot(c, kit.Warning); kit.Line(c, locales.Installed4()+number, kit.Caption) })
			}
			kit.Segmented(c, &p.conflictChoice, locales.ReplaceExisting(), locales.KeepDisabled2())
		}
	})
	versions := p.filteredVersions(data)
	if data.Loading && len(versions) == 0 {
		kit.Loading(c)
		return
	}
	if len(versions) == 0 {
		if data.Error != "" {
			kit.EmptyState(c, kit.Download, locales.CouldNotLoad(), data.Error, func() { kit.Button(c, locales.Retry(), func() { actions.Versions(*project) }) })
		} else {
			kit.EmptyState(c, kit.Cube, locales.NoCompatibleVersions(), locales.AdjustTheGameVersionLoaderOrChannelFilters(), nil)
		}
		return
	}
	ui.Box(c).Height(4).Shrink(0)
	c.Key("resource-versions")
	ui.List(c, &p.versionList, len(versions)+1).Grow(1).FillWidth().MinHeight(0).Margin(0, -8).ItemKey(func(i int) any {
		if i == len(versions) {
			return "versions-end"
		}
		return versions[i].ID
	}).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		if row.Index == len(versions) {
			kit.ListEnd(c, len(versions), false, "", false, nil)
			return
		}
		v := versions[row.Index]
		installed, _, current := installedVersion(data.Entries, v)
		label, badge := locales.Install(), ""
		if installed {
			if current {
				label, badge = locales.Reinstall(), locales.CurrentVersion()
			} else if p.conflictChoice == 0 {
				label = locales.Replace()
			} else {
				label = locales.InstallAlongside()
			}
		}
		if !v.Downloadable && v.Website != "" {
			label = locales.DownloadInBrowser()
		}
		action := func() {
			if !v.Downloadable && v.Website != "" {
				services.OpenURL(v.Website)
			} else {
				p.requestInstall(v, data, actions)
			}
		}
		title := v.Number
		if title == "" {
			title = v.Name
		}
		deps := locales.NoDependencies()
		if len(v.Dependencies) > 0 {
			deps = fmt.Sprintf(locales.Dependencies(), len(v.Dependencies))
		}
		kit.VersionRow(c, kit.VersionRowData{Title: title, Subtitle: kit.MetaLine(v.Name, deps), Channel: typeChannel(v.Channel), Tone: channelTone(v.Channel), Badge: badge, Compatibility: kit.MetaLine(strings.Join(v.GameVersions, " / "), strings.Join(v.Loaders, " / ")), Detail: kit.MetaLine(v.Published.Format("2006-01-02"), formatVersionSize(v.Size)), Action: label, Disabled: data.Busy || data.EntriesLoading}, action)
	})
}

func (p *Resources) versionGameLabel(data *ResourcesData) string {
	value := p.versionCompatibility.GameVersion
	if value == "" || value == resourcecatalog.AnyCompatibility {
		value = locales.All()
	}
	return locales.GameVersion() + " · " + value
}

func (p *Resources) versionLoaderLabel(data *ResourcesData) string {
	value := p.versionCompatibility.Loader
	if value == "" || value == resourcecatalog.AnyCompatibility {
		value = locales.All()
	}
	return locales.Loader() + " · " + loaderLabel(value)
}

func (p *Resources) versionChannelLabel() string {
	if p.versionChannel == "" {
		return locales.Channel() + " · " + locales.All()
	}
	return locales.Channel() + " · " + typeChannel(p.versionChannel)
}

func (p *Resources) filteredVersions(data *ResourcesData) []resourcecatalog.Version {
	filter := p.versionCompatibility.Apply(resourcecatalog.Filter{GameVersion: data.GameVersion, Loaders: []string{loaderID(data.Loader)}})
	game, loader := filter.GameVersion, strings.Join(filter.Loaders, " / ")
	if data.Kind != resource.Mod {
		loader = ""
	}
	if game == "" && loader == "" && p.versionChannel == "" {
		return data.Versions
	}
	filtered := make([]resourcecatalog.Version, 0, len(data.Versions))
	for _, version := range data.Versions {
		if p.versionChannel != "" && version.Channel != p.versionChannel {
			continue
		}
		if game != "" && len(version.GameVersions) > 0 && !slices.Contains(version.GameVersions, game) {
			continue
		}
		if loader != "" && len(version.Loaders) > 0 && !slices.ContainsFunc(version.Loaders, func(value string) bool { return strings.EqualFold(value, loader) }) {
			continue
		}
		filtered = append(filtered, version)
	}
	return filtered
}

func loaderID(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return strings.ToLower(fields[0])
}

func loaderLabel(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return value
	}
	switch strings.ToLower(fields[0]) {
	case "forge":
		return "Forge"
	case "neoforge":
		return "NeoForge"
	case "fabric":
		return "Fabric"
	case "quilt":
		return "Quilt"
	case "liteloader":
		return "LiteLoader"
	default:
		return fields[0]
	}
}

func installedProject(entries []resource.Entry, project resourcecatalog.Project) (bool, string) {
	for _, entry := range entries {
		for _, origin := range entry.Origins {
			if origin.Source == string(project.Source) && origin.ProjectID == project.ID {
				return true, origin.Number
			}
		}
	}
	return false, ""
}

func channelTone(channel string) kit.Tone {
	switch channel {
	case "release":
		return kit.SuccessTone
	case "beta":
		return kit.WarningTone
	case "alpha":
		return kit.Dim
	default:
		return kit.Dim
	}
}

func formatVersionSize(size int64) string {
	if size <= 0 {
		return ""
	}
	if size >= 1<<20 {
		return fmt.Sprintf("%.1f MiB", float64(size)/(1<<20))
	}
	return fmt.Sprintf("%.0f KiB", float64(size)/(1<<10))
}

func installedVersion(entries []resource.Entry, version resourcecatalog.Version) (bool, string, bool) {
	for _, entry := range entries {
		for _, origin := range entry.Origins {
			if origin.Source != string(version.Source) || origin.ProjectID != version.ProjectID {
				continue
			}
			return true, origin.Number, origin.VersionID == version.ID
		}
	}
	return false, "", false
}

func (p *Resources) compatibilityFilter(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	changed := func() { actions.Search(p.query, 0) }
	gameOptions := []kit.SelectOption[string]{{Value: resourcecatalog.AnyCompatibility, Label: locales.All()}}
	seen := make(map[string]bool)
	addGame := func(game string) {
		if game != "" && game != resourcecatalog.AnyCompatibility && !seen[game] {
			gameOptions = append(gameOptions, kit.SelectOption[string]{Value: game, Label: game})
			seen[game] = true
		}
	}
	addGame(data.GameVersion)
	addGame(data.Compatibility.GameVersion)
	for _, version := range data.GameVersions {
		if version.Type == "release" {
			addGame(version.ID)
		}
	}
	kit.FilterSelect(c, &data.Compatibility.GameVersion, compatibilityGameLabel(data), gameOptions, 180, changed).Label(locales.GameVersion())
	if data.Kind == resource.Mod {
		loaderOptions := []kit.SelectOption[string]{{Value: resourcecatalog.AnyCompatibility, Label: locales.All()}}
		for _, loader := range []string{loaderID(data.Loader), "forge", "neoforge", "fabric", "quilt", "liteloader"} {
			if loader != "" && !slices.ContainsFunc(loaderOptions, func(option kit.SelectOption[string]) bool { return option.Value == loader }) {
				loaderOptions = append(loaderOptions, kit.SelectOption[string]{Value: loader, Label: loaderLabel(loader)})
			}
		}
		kit.FilterSelect(c, &data.Compatibility.Loader, compatibilityLabel(data.Compatibility.Loader), loaderOptions, 180, changed).Label(locales.Loader())
	}
}

func compatibilityGameLabel(data *ResourcesData) string {
	value := data.Compatibility.GameVersion
	if value == "" || value == resourcecatalog.AnyCompatibility {
		value = locales.AllVersions()
	}
	return locales.GameVersion() + " · " + value
}

func compatibilityLabel(value string) string {
	switch value {
	case "":
		return locales.FollowTargetInstance()
	case resourcecatalog.AnyCompatibility:
		return locales.All()
	default:
		return loaderLabel(value)
	}
}

func (p *Resources) missing(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	if !data.MissingOpen {
		data.Missing = nil
		p.missingList = ui.ListState{}
		return
	}
	if data.UpdatesOpen {
		return
	}
	kit.Dialog(c, &data.MissingOpen, kit.DialogM, func(back, panel ui.Element) {
		panel.Height(float32(min(510, 280+min(4, len(data.Missing))*56))).MaxHeightPercent(85)
		panel.Children(func() {
			kit.DialogHeader(c, locales.AddMissingResourceFiles(), actions.CloseMissing)
			p.missingBody(c, data, actions)
		})
	})
	if !data.MissingOpen {
		actions.CloseMissing()
	}
}

func (p *Resources) missingBody(c *ui.Context, data *ResourcesData, actions ResourcesActions) {
	services := c.Services()
	kit.Label(c, locales.SelectOrDropDownloadedFiles(), kit.Body).TextColor(kit.Muted).Wrap()
	if data.MissingError != "" {
		kit.ErrorText(c, data.MissingError).MaxLines(3).Ellipsis("…")
	}
	ui.List(c, &p.missingList, len(data.Missing)).FillWidth().Grow(1).MinHeight(0).Padding(0, kit.ScrollbarGutter, 0, 0).ItemKey(func(i int) any { return data.Missing[i].SHA1 + ":" + data.Missing[i].Name }).Scrollbars(false).Rows(func(row ui.ListRow) {
		c := row.Context
		file := data.Missing[row.Index]
		ui.Row(c).FillWidth().Padding(8, 0).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
				kit.Label(c, file.Name, kit.Body).MaxLines(1).Ellipsis("…")
				kit.Label(c, fmt.Sprintf("%.2f MiB", float64(file.Size)/(1<<20)), kit.Body).TextColor(kit.Muted)
			})
			kit.Button(c, locales.DownloadInBrowser(), func() { services.OpenURL(file.Website) }).Disabled(file.Website == "")
		})
	})
	if data.MissingStatus != "" {
		kit.Label(c, data.MissingStatus, kit.Body).TextColor(kit.Muted).MaxLines(2).Ellipsis("…")
	}
	if data.MissingBusy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c).Size(16, 16)
			kit.Label(c, locales.VerifyingFiles(), kit.Body).TextColor(kit.Muted)
		})
	}
	kit.DialogFooter(c, func() {
		kit.Button(c, locales.ChooseFiles(), actions.Supplement).Disabled(data.Busy || data.MissingBusy)
		kit.Primary(c, locales.Retry(), actions.RetryMissing).Disabled(data.Busy || data.MissingBusy)
	})
}

func typeChannel(channel string) string {
	switch channel {
	case "release":
		return locales.Release()
	case "beta":
		return "Beta"
	case "alpha":
		return "Alpha"
	}
	return channel
}
func downloadCount(count int64) string {
	if strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") {
		if count >= 1000000000 {
			return fmt.Sprintf("%.1f B", float64(count)/1e9)
		}
		if count >= 1000000 {
			return fmt.Sprintf("%.1f M", float64(count)/1e6)
		}
		if count >= 1000 {
			return fmt.Sprintf("%.1f K", float64(count)/1e3)
		}
		return fmt.Sprint(count)
	}
	if count >= 100000000 {
		return fmt.Sprintf("%.1f 亿", float64(count)/100000000)
	}
	if count >= 10000 {
		return fmt.Sprintf("%.1f 万", float64(count)/10000)
	}
	return fmt.Sprint(count)
}
