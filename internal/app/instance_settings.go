package app

import (
	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/app/page"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/minecraft"
)

func (a *app) openInstanceSettings(key string) {
	if a.pathBusy || a.instanceInUse(key) {
		return
	}
	instance, ok := a.instanceByKey(key)
	if !ok {
		return
	}
	settings, ok := a.cfg.Instances[key]
	if !ok {
		settings = config.InstanceSettings{Game: a.cfg.Game, Java: a.cfg.Java}
	}
	settings.Java = settings.Java.Clone()
	a.pages.instanceSettings = page.InstanceSettings{}
	a.instanceSettings = page.InstanceSettingsData{
		Open: true, Key: key, Name: instance.Name, Icon: a.icons.bitmaps[key],
		Enabled: settings.Enabled, Isolation: instance.Format == minecraft.InstanceMinecraft,
		Config:    config.Config{Game: settings.Game, Java: settings.Java},
		Inherited: config.Config{Game: a.cfg.Game, Java: a.cfg.Java.Clone()},
	}
	a.loadInstanceIcon(instance)
}

func (a *app) saveInstanceSettings() {
	d := &a.instanceSettings
	if !d.Open || a.pathBusy || a.instanceInUse(d.Key) {
		return
	}
	if a.cfg.Instances == nil {
		a.cfg.Instances = make(map[string]config.InstanceSettings)
	}
	a.cfg.Instances[d.Key] = config.InstanceSettings{Enabled: d.Enabled, Game: d.Config.Game, Java: d.Config.Java}
	a.saveConfig()
}

func (a *app) instanceSettingsView(c *ui.Context) {
	a.pages.instanceSettings.View(c, &a.instanceSettings, a.memory, a.memoryError, page.JavaData{
		Java: a.java, JavaLoading: a.javaLoading, RescanJava: a.rescanJava,
		BrowseJava: func() { a.browseJava(true) },
	}, a.saveInstanceSettings, func() { a.instanceSettings.Open = false })
}
