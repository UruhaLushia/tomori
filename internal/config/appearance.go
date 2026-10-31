package config

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
)

const (
	ColorSystem = iota
	ColorLight
	ColorDark
)

const (
	DefaultAccent     = "#77BBDD"
	DefaultFontWeight = 600
	MaxFontScale      = 150
)

const (
	GPUAuto = iota
	GPUOn
	GPUOff
)

var ZoomSteps = [...]int{75, 80, 90, 100, 110, 125, 150, 175, 200}

func NearestZoom(value int) int {
	if value == 0 {
		return 100
	}
	best := ZoomSteps[0]
	for _, step := range ZoomSteps[1:] {
		if math.Abs(float64(step)-float64(value)) < math.Abs(float64(best)-float64(value)) {
			best = step
		}
	}
	return best
}

type Appearance struct {
	Language    string  `json:"language,omitempty"`
	Navigation  int     `json:"navigation"`
	ColorMode   int     `json:"colorMode"`
	GPU         int     `json:"gpu"`
	ShowFPS     bool    `json:"showFPS"`
	Accent      string  `json:"accent"`
	Font        string  `json:"font"`
	LogFont     string  `json:"logFont"`
	FontScale   int     `json:"fontScale"`
	FontWeight  int     `json:"fontWeight"`
	UIZoom      int     `json:"uiZoom"`
	FontSize    float64 `json:"fontSize,omitempty"` // Legacy configuration.
	LibraryView int     `json:"libraryView,omitempty"`
}

func (a *Appearance) UnmarshalJSON(raw []byte) error {
	type appearance Appearance
	decoded := appearance(*a)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if _, ok := fields["fontScale"]; !ok {
		decoded.FontScale = 0
	}
	*a = Appearance(decoded)
	return nil
}

func (a *Appearance) normalize() {
	if a.LibraryView != 1 {
		a.LibraryView = 0
	}
	if a.Language != "en" && a.Language != "zh-Hans" {
		a.Language = ""
	}
	if a.Navigation < NavSidebar || a.Navigation > NavIconSidebar {
		a.Navigation = NavSidebar
	}
	if a.ColorMode < ColorSystem || a.ColorMode > ColorDark {
		a.ColorMode = ColorSystem
	}
	if a.GPU < GPUAuto || a.GPU > GPUOff {
		a.GPU = GPUAuto
	}
	a.Accent = strings.ToLower(strings.TrimSpace(a.Accent))
	valid := (len(a.Accent) == 7 || len(a.Accent) == 9) && a.Accent[0] == '#'
	if valid {
		_, err := hex.DecodeString(a.Accent[1:])
		valid = err == nil
	}
	if !valid {
		a.Accent = DefaultAccent
	}
	a.Font = strings.TrimSpace(a.Font)
	a.LogFont = strings.TrimSpace(a.LogFont)
	switch a.FontWeight {
	case -1:
		a.FontWeight = 300
	case 0:
		a.FontWeight = DefaultFontWeight
	case 1:
		a.FontWeight = 500
	default:
		if a.FontWeight < 100 || a.FontWeight > 900 || a.FontWeight%100 != 0 {
			a.FontWeight = DefaultFontWeight
		}
	}
	a.UIZoom = NearestZoom(a.UIZoom)
	if a.FontScale == 0 {
		a.FontScale = 100
		if a.FontSize > 0 && !math.IsNaN(a.FontSize) && !math.IsInf(a.FontSize, 0) {
			a.FontScale = int(math.Round(a.FontSize/14*20)) * 5
		}
	}
	a.FontScale = max(75, min(MaxFontScale, int(math.Round(float64(a.FontScale)/5))*5))
	a.FontSize = 0
}
