package page

import (
	"path/filepath"
	"strconv"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/locales"
)

type GeneralData struct {
	Language        *string
	Directories     config.Directories
	DirectoryCounts map[string]int
	DirectoryLocked bool
	Storage         config.Format
	ConfigPath      string
	StorageChanging bool
	StorageError    string
	Logging         *config.Logging
	LogError        string
	Launcher        *config.Launcher
}

type GeneralActions struct {
	LanguageChanged func()
	Migrate         func(config.Format)
	Directories     DirectoryActions
	LogLimitChanged func()
	LauncherChanged func()
}

var logLimitOptions = []kit.SelectOption[int]{
	{Value: 16, Label: "16 MiB"}, {Value: 32, Label: "32 MiB"}, {Value: 64, Label: "64 MiB"},
	{Value: 128, Label: "128 MiB"}, {Value: 256, Label: "256 MiB"}, {Value: 512, Label: "512 MiB"},
	{Value: 1024, Label: "1024 MiB"}, {Value: 2048, Label: "2048 MiB"}, {Value: 4096, Label: "4096 MiB"},
}

func General(c *ui.Context, data GeneralData, actions GeneralActions) {
	ui.Column(c).FillWidth().Shrink(0).Gap(kit.S24).Children(func() {
		if data.Language != nil {
			kit.Group(c, locales.GeneralSettings(), func() {
				kit.SettingRow(c, locales.Language(), "", func() {
					options := []kit.SelectOption[string]{{Value: "", Label: locales.System()}, {Value: "zh-Hans", Label: "简体中文"}, {Value: "en", Label: "English"}}
					label := locales.System()
					for _, option := range options {
						if option.Value == *data.Language {
							label = option.Label
						}
					}
					kit.Select(c, data.Language, label, options, 150, 180, actions.LanguageChanged)
				})
			})
		}
		if data.Launcher != nil {
			kit.Group(c, locales.Launcher(), func() {
				width := float32(160)
				if strings.HasPrefix(strings.ToLower(i18n.Locale()), "en") {
					width = 192
				}
				visibility := []kit.SelectOption[config.LauncherVisibility]{
					{Value: config.LauncherKeepVisible, Label: locales.KeepVisible()},
					{Value: config.LauncherMinimize, Label: locales.Minimize()},
					{Value: config.LauncherHide, Label: locales.Hide()},
				}
				label := visibility[0].Label
				for _, option := range visibility {
					if option.Value == data.Launcher.Visibility {
						label = option.Label
					}
				}
				hint := ""
				if data.Launcher.Visibility == config.LauncherHide {
					hint = locales.RestoreFromTheSystemTrayOrByReopeningTomori()
				}
				kit.SettingRow(c, locales.AfterGameStarts(), hint, func() {
					kit.Select(c, &data.Launcher.Visibility, label, visibility, width, 180, actions.LauncherChanged).Label(locales.LauncherVisibility())
				})
				kit.Divider(c)
				close := []kit.SelectOption[config.LauncherClose]{
					{Value: config.LauncherStopGames, Label: locales.StopGamesAndQuit()},
					{Value: config.LauncherWaitForGames, Label: locales.QuitAfterGamesExit()},
				}
				label = close[0].Label
				if data.Launcher.Close == config.LauncherWaitForGames {
					label = close[1].Label
				}
				kit.SettingRow(c, locales.WhenClosingTomori(), "", func() {
					kit.Select(c, &data.Launcher.Close, label, close, width, 220, actions.LauncherChanged).Label(locales.LauncherExitBehavior())
				})
			})
		}
		directorySettings(c, data.Directories, data.DirectoryCounts, data.DirectoryLocked, actions.Directories)
		if data.Logging != nil {
			kit.Group(c, locales.Logs(), func() {
				kit.SettingRow(c, locales.StorageLimit(), "", func() {
					kit.Select(c, &data.Logging.MaxMiB, strconv.Itoa(data.Logging.MaxMiB)+" MiB", logLimitOptions, 128, 144, actions.LogLimitChanged).Label(locales.LogStorageLimit())
				})
				if data.LogError != "" {
					kit.ErrorText(c, data.LogError)
				}
			})
		}
		kit.Group(c, locales.SettingsStorage(), func() {
			if data.StorageError != "" {
				kit.ErrorText(c, data.StorageError)
			}
			kit.SettingRow(c, locales.StorageFormat(), "", func() {
				kit.Label(c, data.Storage.String(), kit.Body).TextColor(kit.Muted)
				if config.SQLiteAvailable() {
					label, target := locales.SwitchToJson(), config.JSON
					if data.Storage == config.JSON {
						label, target = locales.SwitchToSqLite(), config.SQLite
					}
					if data.StorageChanging {
						label = locales.Migrating()
					}
					kit.Button(c, label, func() { actions.Migrate(target) }).Disabled(data.StorageChanging)
				}
			})
			kit.Divider(c)
			kit.FormValueRow(c, locales.SettingsFile(), data.ConfigPath, func() {
				kit.Ghost(c, locales.OpenFolder(), func() {
					_ = mygo.Shell.OpenPath(filepath.Dir(data.ConfigPath))
				}).Height(kit.TabH())
			})
		})
	})
}
