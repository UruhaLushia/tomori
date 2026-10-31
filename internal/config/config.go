package config

import (
	"maps"
	"slices"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/network"
)

const (
	NavSidebar = iota
	NavTop
	NavIconSidebar
)

// Config is the complete launcher configuration. Workers receive Clone snapshots.
type Config struct {
	Accounts    Accounts                    `json:"accounts"`
	Game        Game                        `json:"game"`
	Directories Directories                 `json:"directories"`
	Java        Java                        `json:"java"`
	Download    Download                    `json:"download"`
	Appearance  Appearance                  `json:"appearance"`
	Window      Window                      `json:"window"`
	Logging     Logging                     `json:"logging"`
	Launcher    Launcher                    `json:"launcher"`
	Instances   map[string]InstanceSettings `json:"instances,omitempty"`
	PlayHistory []PlayRecord                `json:"playHistory,omitempty"`
}

type Logging struct {
	MaxMiB int `json:"maxMiB"`
}

type Window struct {
	X          int  `json:"x"`
	Y          int  `json:"y"`
	Width      int  `json:"width"`
	Height     int  `json:"height"`
	Maximized  bool `json:"maximized"`
	FullScreen bool `json:"fullScreen"`
}

type Accounts struct {
	Selected string            `json:"selected"`
	Entries  []account.Account `json:"entries"`
	Servers  []AuthServer      `json:"servers,omitempty"`
}

type AuthServer struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}
type Game struct {
	MemoryAuto      bool            `json:"memoryAuto"`
	MemoryMiB       int             `json:"memoryMiB"`
	Width           int             `json:"width"`
	Height          int             `json:"height"`
	FullScreen      bool            `json:"fullScreen"`
	ExtraJVM        string          `json:"extraJVM"`
	Isolate         bool            `json:"isolate"`
	ProcessPriority ProcessPriority `json:"processPriority"`
	Execution       Execution       `json:"execution"`
}
type Java struct {
	Mode                string         `json:"mode,omitempty"`
	Path                string         `json:"path"`
	Versions            map[int]string `json:"versions,omitempty"`
	ManualPaths         []string       `json:"manualPaths,omitempty"`
	AutoDownload        bool           `json:"autoDownload"`
	IgnoreCompatibility bool           `json:"ignoreCompatibility,omitempty"`
}

func (j Java) SelectionMode() string {
	if j.Mode != "" {
		return j.Mode
	}
	if j.Path != "" {
		return "custom"
	}
	return "auto"
}

func (j Java) RuntimePath(required int) string {
	switch j.SelectionMode() {
	case "custom":
		return j.Path
	case "version":
		return j.Versions[required]
	default:
		return ""
	}
}

func (j Java) Clone() Java {
	j.Versions = maps.Clone(j.Versions)
	j.ManualPaths = slices.Clone(j.ManualPaths)
	return j
}

type Download struct {
	Proxy              network.Proxy `json:"proxy"`
	CacheDirectory     string        `json:"cacheDirectory"`
	Source             string        `json:"source"`
	Segmented          bool          `json:"segmented"`
	SegmentThresholdMB int           `json:"segmentThresholdMB"`
	Workers            int           `json:"workers"`
	SpeedLimitKiB      int           `json:"speedLimitKiB"`
	Connections        int           `json:"connections"`
	Retries            int           `json:"retries"`
	TimeoutSeconds     int           `json:"timeoutSeconds"`
	CurseForgeKey      string        `json:"curseForgeKey,omitempty"`
}

func defaults() Config {
	return Config{
		Directories: defaultDirectories(),
		Appearance:  Appearance{Accent: DefaultAccent, FontScale: 100, FontWeight: DefaultFontWeight, UIZoom: 100, GPU: GPUAuto},
		Game:        Game{MemoryAuto: true, MemoryMiB: 4096, Width: 854, Height: 480, Isolate: true, ProcessPriority: PriorityNormal, Execution: Execution{TimeoutSeconds: 60}},
		Java:        Java{AutoDownload: true},
		Logging:     Logging{MaxMiB: 256},
		Launcher:    Launcher{Visibility: LauncherKeepVisible, Close: LauncherStopGames},
		Download:    Download{Proxy: network.DefaultProxy(), Source: "official", Segmented: true, SegmentThresholdMB: 8, Workers: 16, Connections: 8, Retries: 3, TimeoutSeconds: 60},
	}
}

func (c *Config) normalize() {
	c.Directories.normalize()
	for _, identity := range c.Accounts.Entries {
		if identity.Kind != account.Yggdrasil {
			continue
		}
		found := slices.ContainsFunc(c.Accounts.Servers, func(s AuthServer) bool { return s.URL == identity.ServerURL })
		if !found {
			c.Accounts.Servers = append(c.Accounts.Servers, AuthServer{Name: identity.ServerName, URL: identity.ServerURL})
		}
	}
	if _, ok := c.SelectedAccount(); !ok {
		c.Accounts.Selected = ""
	}
	c.Game.normalize()
	for key, instance := range c.Instances {
		instance.Game.normalize()
		c.Instances[key] = instance
	}
	c.Appearance.normalize()
	c.Launcher.normalize()
	c.Logging.MaxMiB = max(16, min(c.Logging.MaxMiB, 4096))
	if c.Download.Source != "bmclapi" {
		c.Download.Source = "official"
	}
	c.Download.Workers = max(1, min(c.Download.Workers, 32))
	c.Download.SpeedLimitKiB = max(0, min(c.Download.SpeedLimitKiB, 1<<20))
	c.Download.SegmentThresholdMB = max(1, min(c.Download.SegmentThresholdMB, 1024))
	c.Download.Connections = max(1, min(c.Download.Connections, 32))
	c.Download.Retries = max(0, min(c.Download.Retries, 10))
	c.Download.TimeoutSeconds = max(10, min(c.Download.TimeoutSeconds, 600))
}

func (c Config) Clone() Config {
	c.PlayHistory = slices.Clone(c.PlayHistory)
	c.Accounts.Entries = slices.Clone(c.Accounts.Entries)
	c.Accounts.Servers = slices.Clone(c.Accounts.Servers)
	c.Directories.Entries = slices.Clone(c.Directories.Entries)
	c.Directories.InstanceOrder = slices.Clone(c.Directories.InstanceOrder)
	c.Instances = maps.Clone(c.Instances)
	c.Java = c.Java.Clone()
	for key, settings := range c.Instances {
		settings.Java = settings.Java.Clone()
		c.Instances[key] = settings
	}
	return c
}

func (c Config) SelectedAccount() (account.Account, bool) {
	for _, a := range c.Accounts.Entries {
		if a.ID != "" && a.ID == c.Accounts.Selected {
			return a, true
		}
	}
	return account.Account{}, false
}
