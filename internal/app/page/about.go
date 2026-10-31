package page

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"path/filepath"
	"runtime"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

func About(c *ui.Context, configPath string) {
	ui.Column(c).FillWidth().Shrink(0).Gap(24).Children(func() {
		kit.Group(c, locales.Apply(), func() {
			ui.Row(c).FillWidth().Padding(16, 0).Gap(12).AlignItems(ui.Center).Children(func() {
				kit.InstanceIcon(c, nil, 48)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					kit.Label(c, "Tomori", kit.Strong)
					kit.Label(c, locales.EvenWhenLostTheLightRemains(), kit.Caption)
				})
			})
		})
		kit.Group(c, locales.Information(), func() {
			kit.FormValueRow(c, locales.Version(), "0.1.0", nil)
			kit.Divider(c)
			kit.FormValueRow(c, locales.Environment(), runtime.GOOS+" · "+runtime.GOARCH, nil)
			kit.Divider(c)
			kit.FormValueRow(c, locales.SettingsFile(), configPath, func() {
				kit.Ghost(c, locales.OpenFolder(), func() { _ = mygo.Shell.OpenPath(filepath.Dir(configPath)) }).Height(kit.TabH())
			})
		})
		kit.Group(c, locales.Credits(), func() {
			creditRow(c, "mygo", locales.NativeUiFramework(), "https://github.com/egoist/mygo")
			kit.Divider(c)
			creditRow(c, "piko", locales.ParallelRangesForLargeFiles(), "https://github.com/UruhaLushia/piko")
			kit.Divider(c)
			creditRow(c, "Modrinth", locales.ModAndResourcePlatform(), "https://modrinth.com")
			kit.Divider(c)
			creditRow(c, "CurseForge", locales.ModAndResourcePlatform(), "https://www.curseforge.com/minecraft")
			kit.Divider(c)
			creditRow(c, "MC 百科 / HMCL", locales.ChineseModNameCatalog(), "https://www.mcmod.cn")
			kit.Divider(c)
			creditRow(c, "BMCLAPI", locales.DownloadMirrorBangbang93(), "https://bmclapi2.bangbang93.com")
			kit.Divider(c)
			creditRow(c, "Mojang", locales.VersionManifestAndGameFiles(), "https://www.minecraft.net")
		})
		kit.Group(c, locales.Legal(), func() {
			kit.FormValueRow(c, locales.License(), "GPL-3.0", nil)
			kit.Divider(c)
			kit.FormValueRow(c, locales.Copyright(), "© 2026 Tomori", nil)
		})
	})
}

func creditRow(c *ui.Context, name, desc, url string) {
	kit.FormRow(c, name, desc, func() {
		kit.IconButton(c, kit.External, false, func() { _ = mygo.Shell.OpenExternal(url) }).Size(28, 28).Label(locales.OpenWebsite()).Tooltip(locales.OpenWebsite())
	})
}
