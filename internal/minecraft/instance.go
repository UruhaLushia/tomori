package minecraft

import (
	"path/filepath"
	"runtime"
	"strings"
)

type InstanceFormat string

type PackSource struct {
	Provider  string `json:"provider"`
	ProjectID string `json:"project"`
	VersionID string `json:"version"`
	Number    string `json:"number,omitempty"`
}

const (
	InstanceMinecraft InstanceFormat = "minecraft"
	InstancePrism     InstanceFormat = "prism"
	InstanceManaged   InstanceFormat = "tomori"
)

// Instance describes a local installation independently of a remote version.
type Instance struct {
	VersionSummary
	Name, GameVersion, Loader            string
	GameDir, InstanceDir                 string
	SourceDir, SourceName                string
	Format                               InstanceFormat
	Loaders                              []string
	Description, PackFormat, PackVersion string
	IconKey                              string
	PackSource                           PackSource
}

func (i Instance) CanLaunch() bool {
	return i.Format == InstanceMinecraft || i.Format == InstanceManaged || i.Format == InstancePrism
}

func (i Instance) ContentDirectory(isolate bool) string {
	if i.Format == InstanceMinecraft && isolate {
		return i.InstanceDir
	}
	return i.GameDir
}

// Key identifies an installation, including when directories contain the same version.
func (i Instance) Key() string {
	path := filepath.Clean(i.InstanceDir)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return string(i.Format) + ":" + path
}
