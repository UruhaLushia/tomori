package minecraft

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type VersionDetail struct {
	ID                     string         `json:"id"`
	Jar                    string         `json:"jar,omitempty"`
	InheritsFrom           string         `json:"inheritsFrom,omitempty"`
	MinecraftVersion       string         `json:"minecraftVersion,omitempty"`
	Type                   string         `json:"type"`
	MainClass              string         `json:"mainClass"`
	Assets                 string         `json:"assets"`
	AssetIndex             AssetIndex     `json:"assetIndex"`
	Downloads              Downloads      `json:"downloads"`
	JavaVersion            *JavaTarget    `json:"javaVersion"`
	CompatibleJavaMajors   []int          `json:"compatibleJavaMajors,omitempty"`
	Libraries              []Library      `json:"libraries"`
	MavenFiles             []Library      `json:"mavenFiles,omitempty"` // Installer dependencies, excluded from the game classpath.
	Patches                []VersionPatch `json:"patches,omitempty"`
	Arguments              Arguments      `json:"arguments"`
	MinecraftArguments     string         `json:"minecraftArguments"`
	Logging                *Logging       `json:"logging"`
	ComplianceLevel        int            `json:"complianceLevel"`
	MinimumLauncherVersion int            `json:"minimumLauncherVersion"`
	ReleaseTime            time.Time      `json:"releaseTime"`
	Time                   time.Time      `json:"time"`
}

type AssetIndex struct {
	ID        string `json:"id"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	TotalSize int64  `json:"totalSize"`
	URL       string `json:"url"`
}

type Downloads struct {
	Client         FileRef `json:"client"`
	Server         FileRef `json:"server"`
	ClientMappings FileRef `json:"client_mappings"`
}

type FileRef struct {
	Name string `json:"id,omitempty"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Path string `json:"path"`
}

type JavaTarget struct {
	Component    string `json:"component"`
	MajorVersion int    `json:"majorVersion"`
}

type Library struct {
	Name      string            `json:"name"`
	URL       string            `json:"url,omitempty"`
	SHA1      string            `json:"sha1,omitempty"`
	Size      int64             `json:"size,omitempty"`
	Downloads LibraryDownloads  `json:"downloads"`
	Natives   map[string]string `json:"natives"`
	Rules     []Rule            `json:"rules"`
	Extract   *LibraryExtract   `json:"extract"`
}

type LibraryDownloads struct {
	Artifact    *FileRef           `json:"artifact"`
	Classifiers map[string]FileRef `json:"classifiers"`
}

type LibraryExtract struct {
	Exclude []string `json:"exclude"`
}

type Arguments struct {
	Game []Argument `json:"game"`
	JVM  []Argument `json:"jvm"`
}

type Argument struct {
	Rules []Rule        `json:"rules"`
	Value ArgumentValue `json:"value"`
}

type ArgumentValue []string

func (a *Argument) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var value string
		if err := json.Unmarshal(b, &value); err != nil {
			return err
		}
		a.Rules, a.Value = nil, ArgumentValue{value}
		return nil
	}
	type argument Argument
	return json.Unmarshal(b, (*argument)(a))
}

func (v *ArgumentValue) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = []string{s}
		return nil
	}
	var list []string
	if err := json.Unmarshal(b, &list); err != nil {
		return err
	}
	*v = list
	return nil
}

type Logging struct {
	Client *LoggingClient `json:"client"`
}

type LoggingClient struct {
	Argument string  `json:"argument"`
	File     FileRef `json:"file"`
}

func (f FileRef) ID() string {
	if f.Name != "" {
		return filepath.Base(f.Name)
	}
	if f.Path != "" {
		return filepath.Base(f.Path)
	}
	if f.URL != "" {
		return filepath.Base(f.URL)
	}
	return "client"
}

func ReadLocalVersion(gameDir, id string) (*VersionDetail, error) {
	root, err := os.OpenRoot(gameDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return readLocalVersion(root, id, make(map[string]bool))
}

func readLocalVersion(root *os.Root, id string, parents map[string]bool) (*VersionDetail, error) {
	if err := fsutil.ValidateID(id); err != nil {
		return nil, err
	}
	if parents[id] || len(parents) >= 32 {
		return nil, fmt.Errorf(locales.CircularOrExcessiveVersionInheritance(), id)
	}
	parents[id] = true
	raw, err := fsutil.ReadRootFile(root, filepath.Join("versions", id, id+".json"), 16<<20)
	if err != nil {
		return nil, err
	}
	var detail VersionDetail
	if err := json.Unmarshal(raw, &detail); err != nil {
		return nil, err
	}
	if detail.ID == "" {
		detail.ID = id
	}
	if detail.ID != id {
		return nil, fmt.Errorf(locales.VersionDirectoryMetadataIdMismatch(), id, detail.ID)
	}
	patches := detail.Patches
	detail.Patches = nil
	if detail.InheritsFrom != "" {
		base, err := readLocalVersion(root, detail.InheritsFrom, parents)
		if err != nil {
			return nil, fmt.Errorf(locales.ReadingBaseVersionFor(), id, err)
		}
		detail = *MergeVersion(base, &detail)
	} else {
		if len(patches) > 0 {
			// The top-level launch fields may already be flattened by another
			// launcher. Rebuild once from the authoritative component patches.
			detail = VersionDetail{ID: id, Jar: detail.Jar, MinecraftVersion: detail.MinecraftVersion, Time: detail.Time, ReleaseTime: detail.ReleaseTime}
		}
		if detail.Jar == "" {
			detail.Jar = id
		}
	}
	if len(patches) > 0 {
		detail = *ApplyVersionPatches(&detail, patches)
	}
	if err := detail.Validate(); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (d *VersionDetail) Validate() error {
	if d == nil {
		return errors.New(locales.VersionInformationIsEmpty())
	}
	if err := fsutil.ValidateID(d.ID); err != nil {
		return err
	}
	if d.Jar != "" {
		if err := fsutil.ValidateID(d.Jar); err != nil {
			return err
		}
	}
	if d.InheritsFrom != "" {
		return fmt.Errorf(locales.BaseVersionForIsUnresolved(), d.ID, d.InheritsFrom)
	}
	if d.MainClass == "" {
		return fmt.Errorf(locales.HasNoMainClass(), d.ID)
	}
	if d.AssetIndex.URL != "" {
		if err := fsutil.ValidateID(d.AssetIndex.ID); err != nil {
			return err
		}
	}
	return nil
}

func (d *VersionDetail) ClientID() string {
	if d.Jar != "" {
		return d.Jar
	}
	return d.ID
}

func (d *VersionDetail) RequiredJava() int {
	if d.JavaVersion != nil && d.JavaVersion.MajorVersion > 0 {
		return d.JavaVersion.MajorVersion
	}
	// Older third-party profiles may omit javaVersion while retaining the
	// original client version. Do not default every modern release to Java 8.
	version, _, _ := strings.Cut(ProfileGameVersion(d, ""), "-")
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 8
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	if majorErr != nil || minorErr != nil {
		return 8
	}
	if major >= 26 {
		return 25
	}
	if major == 1 {
		patch := 0
		if len(parts) > 2 {
			patch, _ = strconv.Atoi(parts[2])
		}
		switch {
		case minor > 20 || minor == 20 && patch >= 5:
			return 21
		case minor >= 18:
			return 17
		case minor == 17:
			return 16
		}
	}
	return 8
}
