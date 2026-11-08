package page

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/kit"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	proxyconfig "github.com/UruhaLushia/tomori/internal/network/proxy"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type DownloadData struct {
	VersionCount   int
	InstalledCount int
	CacheBusy      bool
	CacheError     string
	CacheStatus    string
	CacheSize      uint64
	CacheSizeKnown bool
}

type DownloadActions struct {
	SourceChanged func()
	Reload        func()
	Save          func()
	ChangeCache   func()
	ClearCache    func()
	OpenCache     func()
}

type Download struct {
	source      minecraft.Source
	sourceIndex int
	workers     numberSetting
	speedLimit  numberSetting
	connections numberSetting
	threshold   numberSetting
	retries     numberSetting
	timeout     numberSetting
	proxyPort   numberSetting
}

func (p *Download) View(c *ui.Context, cfg *config.Config, data DownloadData, actions DownloadActions) {
	if source := minecraft.ParseSource(cfg.Download.Source); source != p.source {
		p.source = source
		p.sourceIndex = 0
		if source == minecraft.SourceMirror {
			p.sourceIndex = 1
		}
	}
	ui.Column(c).FillWidth().Shrink(0).Gap(24).Children(func() {
		kit.FormGroup(c, locales.Source(), nil, func() {
			kit.FormRow(c, locales.Source(), "", func() {
				kit.Segmented(c, &p.sourceIndex, locales.Mojang(), "BMCLAPI").OnChange(func() {
					p.source = minecraft.SourceOfficial
					if p.sourceIndex == 1 {
						p.source = minecraft.SourceMirror
					}
					cfg.Download.Source = string(p.source)
					actions.SourceChanged()
				})
			})
			kit.Divider(c)
			kit.FormValueRow(c, locales.VersionManifest(), fmt.Sprintf(locales.VersionsInstalled2(), data.VersionCount, data.InstalledCount), func() { kit.Ghost(c, locales.Reload(), actions.Reload).Height(kit.TabH()) })
		})
		kit.FormGroup(c, locales.Concurrency(), nil, func() {
			kit.FormRow(c, locales.ConcurrentFiles(), "", func() { p.workers.input(c, &cfg.Download.Workers, 1, 32, 1, actions.Save) })
			kit.Divider(c)
			kit.FormRow(c, locales.Connections(), "", func() { p.connections.input(c, &cfg.Download.Connections, 1, 32, 1, actions.Save) })
			kit.Divider(c)
			kit.FormRow(c, locales.SegmentedDownloads(), "", func() { kit.Switch(c, &cfg.Download.Segmented).OnChange(actions.Save) })
			kit.Divider(c)
			kit.FormRow(c, locales.SegmentThresholdMiB(), "", func() {
				ui.Box(c).Disabled(!cfg.Download.Segmented).Children(func() { p.threshold.input(c, &cfg.Download.SegmentThresholdMB, 1, 1024, 1, actions.Save) })
			})
		})
		kit.FormGroup(c, locales.Limits(), nil, func() {
			kit.FormRow(c, locales.SpeedLimit(), locales.KiBS0ForUnlimited(), func() { p.speedLimit.input(c, &cfg.Download.SpeedLimitKiB, 0, 1<<20, 128, actions.Save) })
			kit.Divider(c)
			kit.FormRow(c, locales.RequestTimeoutS(), "", func() { p.timeout.input(c, &cfg.Download.TimeoutSeconds, 10, 600, 10, actions.Save) })
			kit.Divider(c)
			kit.FormRow(c, locales.Retries(), "", func() { p.retries.input(c, &cfg.Download.Retries, 0, 10, 1, actions.Save) })
		})
		p.proxySettings(c, &cfg.Download.Proxy, actions.Save)
		kit.FormGroup(c, locales.Cache(), nil, func() {
			kit.FormValueRow(c, locales.CacheDirectory(), cfg.CacheDirectory(), func() {
				kit.Ghost(c, locales.Change(), actions.ChangeCache).Height(kit.TabH()).Disabled(data.CacheBusy)
				kit.IconButton(c, kit.Folder, false, actions.OpenCache).Size(28, 28).Label(locales.OpenCacheFolder()).Tooltip(locales.OpenCacheFolder())
			})
			kit.Divider(c)
			kit.FormRow(c, locales.ClearCache(), data.CacheStatus, func() {
				if data.CacheSizeKnown {
					kit.Line(c, fmt.Sprintf(locales.Used(), storage.Size(data.CacheSize)), kit.BodyMuted)
				}
				kit.Button(c, locales.Clear(), actions.ClearCache).Height(kit.TabH()).Disabled(data.CacheBusy)
			})
			if data.CacheError != "" {
				kit.ErrorText(c, data.CacheError)
			}
		})
		kit.FormGroup(c, "API", nil, func() {
			kit.FormRow(c, "CurseForge API Key", "", func() {
				kit.Input(c, &cfg.Download.CurseForgeKey).Password().Placeholder(locales.CustomApiKeyOptional()).Width(kit.W(240)).Height(kit.ControlH()).OnChange(actions.Save)
			})
		})
	})
}

func (p *Download) proxySettings(c *ui.Context, proxy *proxyconfig.Proxy, save func()) {
	kit.Group(c, locales.ProxyLabel(), func() {
		kit.SettingRow(c, locales.ProxyMode(), "", func() {
			c.Key("proxy-mode")
			kit.SegmentedValues(c, &proxy.Mode, []kit.SelectOption[string]{{Value: proxyconfig.System, Label: locales.SystemProxy()}, {Value: proxyconfig.Direct, Label: locales.Direct()}, {Value: proxyconfig.Manual, Label: locales.ManualProxy()}}, save).Label(locales.ProxyMode())
		})
		if proxy.Mode != proxyconfig.Manual {
			return
		}
		kit.FoldMotion(ui.Column(c.Key("proxy-fields")).FillWidth()).Children(func() {
			kit.Divider(c)
			kit.SettingRow(c, locales.Protocol(), "", func() {
				c.Key("proxy-protocol")
				kit.Select(c, &proxy.Protocol, map[string]string{"http": "HTTP", "https": "HTTPS", "socks5": "SOCKS5", "socks5h": "SOCKS5"}[proxy.Protocol], []kit.SelectOption[string]{
					{Value: "http", Label: "HTTP"},
					{Value: "https", Label: "HTTPS"},
					{Value: "socks5", Label: "SOCKS5"},
				}, 100, 120, save).Label(locales.ProxyProtocol())
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.Address(), "", func() {
				c.Key("proxy-host")
				kit.Input(c, &proxy.Host).Width(kit.W(240)).Placeholder("127.0.0.1").Label(locales.ProxyAddress()).OnChange(save)
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.Port(), "", func() {
				c.Key("proxy-port")
				p.proxyPort.input(c, &proxy.Port, 1, 65535, 1, save)
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.Username(), "", func() {
				c.Key("proxy-username")
				kit.Input(c, &proxy.Username).Width(kit.W(240)).Placeholder(locales.Optional()).Label(locales.ProxyUsername()).OnChange(save)
			})
			kit.Divider(c)
			kit.SettingRow(c, locales.Password(), "", func() {
				c.Key("proxy-password")
				kit.Input(c, &proxy.Password).Width(kit.W(240)).Password().Placeholder(locales.Optional()).Label(locales.ProxyPassword()).OnChange(save)
			})
			if err := proxy.Validate(); err != nil {
				kit.ErrorText(c, err.Error())
			}
		})
	})
}
