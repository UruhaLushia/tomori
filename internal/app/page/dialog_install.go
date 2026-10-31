package page

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/loader"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type InstallDialog struct {
	open                       bool
	name, nameVersion, version string
	options                    []kit.SelectOption[string]
	query                      string
	selected                   int
	filtered                   []minecraft.VersionSummary
	matches                    []minecraft.VersionSummary
	filterSource               *minecraft.VersionSummary
	filterCount                int
	filterQuery                string
	directory                  string
	loader                     string
	loaderVersion              string
}

type LoaderCatalogData struct {
	Game, Kind, Error string
	Loading           bool
	Versions          []loader.Version
}

func (p *InstallDialog) refreshLoaders(actions InstancesActions) {
	p.loaderVersion = ""
	if v, ok := p.target(); ok && actions.Loaders != nil {
		actions.Loaders(v.ID, p.loader)
	}
}

func (p *InstallDialog) View(c *ui.Context, data InstancesData, actions InstancesActions) {
	if !p.open {
		if actions.AssessInstall != nil {
			actions.AssessInstall("", "", modpack.Loader{})
		}
		return
	}
	p.filterVersions(data.Versions)
	p.syncVersion()
	var targetDirectory config.GameDirectory
	for _, entry := range data.InstallDirectories {
		if entry.Path == p.directory {
			targetDirectory = entry
			break
		}
	}
	if targetDirectory.Path == "" && len(data.InstallDirectories) > 0 {
		targetDirectory = data.InstallDirectories[0]
		p.directory = targetDirectory.Path
	}
	if v, ok := p.target(); ok && p.loader != "" && (data.Loaders.Game != v.ID || data.Loaders.Kind != p.loader) {
		p.refreshLoaders(actions)
	}
	kit.Dialog(c, &p.open, kit.DialogS, func(back, panel ui.Element) {
		panel.Children(func() {
			kit.DialogHeader(c, locales.NewInstance(), func() { p.open = false }).Label(locales.CloseInstallation())
			ui.Scroll(c).Scrollbars(false).FillWidth().MinHeight(0).Shrink(1).Children(func() {
				ui.Column(c).FillWidth().Shrink(0).Gap(16).Children(func() {
					kit.Field(c, locales.GameVersion(), func() {
						c.Key("install-version")
						kit.Select(c, &p.version, p.version, p.options, 200, 320, func() {
							p.syncVersion()
							p.refreshLoaders(actions)
						}).FillWidth().Disabled(data.Busy || len(p.options) == 0).Label(locales.GameVersion())
						if data.ManifestLoading {
							c.Key("manifest-loading")
							ui.Spinner(c).Size(16, 16)
						}
					})
					kit.Field(c, locales.InstanceName(), func() {
						c.Key("install-name")
						kit.Input(c, &p.name).FillWidth().Label(locales.InstanceName()).Disabled(data.Busy)
					})
					p.loaderInput(c, data, actions)
					kit.Field(c, locales.InstallLocation(), func() {
						options := make([]kit.SelectOption[string], 0, len(data.InstallDirectories))
						for _, entry := range data.InstallDirectories {
							options = append(options, kit.SelectOption[string]{Value: entry.Path, Label: entry.Name})
						}
						c.Key("install-directory")
						kit.Select(c, &p.directory, targetDirectory.Name, options, 200, 320, nil).FillWidth().Disabled(data.Busy).Label(locales.InstallLocation())
					})
					if data.ManifestError != "" {
						kit.ErrorText(c, data.ManifestError)
					}
					if v, ok := p.target(); ok && p.open && actions.AssessInstall != nil {
						data.Space = actions.AssessInstall(v.ID, p.directory, modpack.Loader{ID: p.loader, Version: p.loaderVersion})
						SpaceSummary(c, data.Space)
					}
				})
			})
			kit.DialogFooter(c, func() {
				kit.Ghost(c, locales.Cancel(), func() { p.open = false })
				v, ok := p.target()
				name := strings.TrimSpace(p.name)
				valid := name != "" && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 128 && !strings.ContainsFunc(name, unicode.IsControl)
				kit.Primary(c, locales.Install(), func() {
					p.open = false
					actions.Install(v.ID, p.directory, modpack.Loader{ID: p.loader, Version: p.loaderVersion}, name)
				}).Disabled(data.Busy || data.Space.Blocked() || !ok || !valid || targetDirectory.Path == "" || !p.loaderReady(data.Loaders, v.ID))
			})
		})
	})
}

func (p *InstallDialog) syncVersion() {
	if p.version == "" && len(p.filtered) > 0 {
		p.version = p.filtered[0].ID
	}
	p.selected = -1
	for i, version := range p.filtered {
		if version.ID == p.version {
			p.selected = i
			break
		}
	}
	if p.nameVersion != p.version {
		if p.name == p.nameVersion {
			p.name = p.version
		}
		p.nameVersion = p.version
	}
}

func (p *InstallDialog) target() (minecraft.VersionSummary, bool) {
	if p.selected < 0 || p.selected >= len(p.filtered) {
		return minecraft.VersionSummary{}, false
	}
	return p.filtered[p.selected], true
}

func (p *InstallDialog) show() {
	p.open = true
	p.name, p.nameVersion, p.version = "", "", ""
	p.query = ""
	p.selected = 0
	p.directory = config.DefaultGameDirectory()
	p.loader, p.loaderVersion = "", ""
}

func (p *InstallDialog) loaderReady(data LoaderCatalogData, game string) bool {
	if p.loader == "" {
		return true
	}
	if data.Loading || data.Error != "" || data.Game != game || data.Kind != p.loader {
		return false
	}
	for _, version := range data.Versions {
		if version.ID == p.loaderVersion {
			return true
		}
	}
	return false
}

func (p *InstallDialog) loaderInput(c *ui.Context, data InstancesData, actions InstancesActions) {
	options := []kit.SelectOption[string]{{Value: "", Label: locales.Vanilla()}, {Value: "fabric", Label: "Fabric"}, {Value: "quilt", Label: "Quilt"}, {Value: "forge", Label: "Forge"}, {Value: "neoforge", Label: "NeoForge"}}
	kit.Field(c, locales.Loader(), func() {
		c.Key("install-loader")
		kit.SegmentedValues(c, &p.loader, options, func() { p.refreshLoaders(actions) }).FillWidth().Disabled(data.Busy).Label(locales.Loader())
	})
	if p.loader == "" {
		return
	}
	kit.Field(c, locales.LoaderVersion(), func() {
		d := data.Loaders
		v, ok := p.target()
		switch {
		case !ok || d.Game != v.ID || d.Kind != p.loader || d.Loading:
			c.Key("loader-loading")
			ui.Spinner(c).Size(16, 16)
		case d.Error != "":
			kit.Ghost(c, locales.Retry(), func() { p.refreshLoaders(actions) }).Height(kit.TabH())
		case len(d.Versions) == 0:
			kit.Label(c, locales.NoCompatibleVersions(), kit.Caption)
		default:
			if p.loaderVersion == "" {
				p.loaderVersion = d.Versions[0].ID
			}
			versions := make([]kit.SelectOption[string], 0, len(d.Versions))
			for _, version := range d.Versions {
				versions = append(versions, kit.SelectOption[string]{Value: version.ID, Label: version.ID})
			}
			c.Key("install-loader-version")
			kit.Select(c, &p.loaderVersion, p.loaderVersion, versions, 200, 320, nil).FillWidth().Disabled(data.Busy).Label(locales.LoaderVersion())
		}
	})
	if data.Loaders.Kind == p.loader && data.Loaders.Error != "" {
		kit.ErrorText(c, data.Loaders.Error)
	}
}

func (p *InstallDialog) filterVersions(versions []minecraft.VersionSummary) {
	query := strings.ToLower(strings.TrimSpace(p.query))
	var source *minecraft.VersionSummary
	if len(versions) > 0 {
		source = &versions[0]
	}
	if source != p.filterSource || len(versions) != p.filterCount || query != p.filterQuery {
		p.filterSource, p.filterCount, p.filterQuery = source, len(versions), query
		p.filtered = versions
		if query != "" {
			p.matches = p.matches[:0]
			for _, v := range versions {
				if strings.Contains(strings.ToLower(v.ID), query) {
					p.matches = append(p.matches, v)
				}
			}
			p.filtered = p.matches
		}
		p.options = make([]kit.SelectOption[string], 0, len(p.filtered))
		for _, version := range p.filtered {
			p.options = append(p.options, kit.SelectOption[string]{Value: version.ID, Label: version.ID})
		}
	}
	p.selected = max(0, min(p.selected, len(p.filtered)-1))
}

func (p *InstallDialog) Open(version string) { p.show(); p.version = version }
