package app

import (
	"slices"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *app) saveConfig() {
	if a.downloadBudget != nil {
		a.downloadBudget.SetSpeedLimit(int64(a.cfg.Download.SpeedLimitKiB) << 10)
	}
	if err := a.persistConfig(); err != nil {
		a.logf(locales.FailedToSaveSettings(), err)
		a.notifyError(locales.CouldNotSaveSettings(), err)
	}
	if !slices.Equal(a.settingsDirectories, a.cfg.Directories.Entries) {
		a.settingsDirectories = slices.Clone(a.cfg.Directories.Entries)
		a.installed = map[string]bool{}
		a.installedList = nil
		a.scanInstalled()
	}
}

func (a *app) persistConfig() error {
	if a.storageChanging {
		return nil
	}
	return a.store.Save(a.cfg.Clone())
}

func (a *app) changeStorage(format config.Format) {
	if a.storageChanging || format == a.store.Format() {
		return
	}
	a.storageChanging = true
	a.storageError = ""
	snapshot := a.cfg.Clone()
	a.workers.Go(func() {
		err := a.store.Migrate(format, snapshot)
		a.win.Update(func() {
			a.storageChanging = false
			if err != nil {
				a.storageError = err.Error()
				a.logf(locales.FailedToMigrateSettings(), err)
			} else {
				a.logf(locales.SettingsMigratedTo(), format)
			}
			a.saveConfig()
		})
	})
}

func (a *app) sourceChanged() {
	a.saveConfig()
	a.loadManifest()
}

func (a *app) selectVersion(id string) {
	instance, ok := a.instanceByKey(id)
	if !ok || !instance.CanLaunch() {
		return
	}
	a.cfg.Directories.SelectedInstance = id
	a.home.failureKey, a.home.failure, a.home.failureLog = "", "", ""
	a.saveConfig()
}
