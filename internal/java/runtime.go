package java

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

const runtimeManifestURL = "https://launchermeta.mojang.com/v1/products/java-runtime/2ec0cc96c44e5a76b9c8b7c39df7210883d12871/all.json"

type Distribution string

const (
	DistributionMojang           Distribution = "mojang"
	DistributionTemurin          Distribution = "temurin"
	DistributionZulu             Distribution = "zulu"
	DistributionLiberica         Distribution = "liberica"
	DistributionCorretto         Distribution = "corretto"
	DistributionMicrosoft        Distribution = "microsoft"
	DistributionSapMachine       Distribution = "sap_machine"
	DistributionSemeru           Distribution = "semeru"
	DistributionOpenLogic        Distribution = "openlogic"
	DistributionOracleOpenJDK    Distribution = "oracle_open_jdk"
	DistributionGraalVMCommunity Distribution = "graalvm_community"
	DistributionDragonwell       Distribution = "dragonwell"
	DistributionKona             Distribution = "kona"
	DistributionJetBrains        Distribution = "jetbrains"
)

type PackageType string

const (
	PackageJRE PackageType = "jre"
	PackageJDK PackageType = "jdk"
)

type DownloadRequest struct {
	Distribution Distribution
	PackageType  PackageType
	Major        int
	Component    string
}

type DistributionInfo struct {
	Value  Distribution
	Label  string
	HasJRE bool
}

func SupportedDistributions() []DistributionInfo {
	return []DistributionInfo{
		{Value: DistributionMojang, Label: "Mojang", HasJRE: true},
		{Value: DistributionTemurin, Label: "Temurin", HasJRE: true},
		{Value: DistributionZulu, Label: "Zulu", HasJRE: true},
		{Value: DistributionLiberica, Label: "Liberica", HasJRE: true},
		{Value: DistributionCorretto, Label: "Corretto"},
		{Value: DistributionMicrosoft, Label: "Microsoft"},
		{Value: DistributionSapMachine, Label: "SapMachine", HasJRE: true},
		{Value: DistributionSemeru, Label: "Semeru", HasJRE: true},
		{Value: DistributionOpenLogic, Label: "OpenLogic", HasJRE: true},
		{Value: DistributionOracleOpenJDK, Label: "Oracle OpenJDK"},
		{Value: DistributionGraalVMCommunity, Label: "GraalVM Community"},
		{Value: DistributionDragonwell, Label: "Dragonwell"},
		{Value: DistributionKona, Label: "Kona"},
		{Value: DistributionJetBrains, Label: "JetBrains"},
	}
}

func DistributionInfoFor(value Distribution) (DistributionInfo, bool) {
	for _, info := range SupportedDistributions() {
		if info.Value == value {
			return info, true
		}
	}
	return DistributionInfo{}, false
}

func Managed(ctx context.Context, dir string) []Runtime {
	markers, _ := filepath.Glob(filepath.Join(dir, "*", "*", ".complete"))
	var list []Runtime
	for _, marker := range markers {
		if ctx.Err() != nil {
			break
		}
		root := filepath.Dir(marker)
		if filepath.Base(root) != javaPlatform() {
			continue
		}
		for _, path := range []string{filepath.Join(root, "bin", javaBinary()), filepath.Join(root, "jre.bundle", "Contents", "Home", "bin", javaBinary())} {
			if info, err := inspectCached(ctx, path); err == nil {
				list = append(list, info)
				break
			}
		}
	}
	return list
}

func ManagedRoot(dir, executable string) string {
	executable = filepath.Clean(executable)
	markers, _ := filepath.Glob(filepath.Join(dir, "*", "*", ".complete"))
	for _, marker := range markers {
		root := filepath.Dir(marker)
		if filepath.Base(root) != javaPlatform() {
			continue
		}
		for _, path := range []string{filepath.Join(root, "bin", javaBinary()), filepath.Join(root, "jre.bundle", "Contents", "Home", "bin", javaBinary())} {
			if filepath.Clean(path) == executable {
				return filepath.Dir(root)
			}
		}
	}
	return ""
}

func RemoveManaged(ctx context.Context, dir, executable string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root := ManagedRoot(dir, executable)
	if root == "" {
		return errors.New(locales.JavaRuntimeIsNotManaged())
	}
	return os.RemoveAll(root)
}

func SupportedMajors() []int {
	return []int{8, 16, 17, 21, 25}
}

func Ensure(ctx context.Context, f *download.Client, d *minecraft.VersionDetail, list []Runtime, custom, dir string, auto, ignoreCompatibility bool, progress func(minecraft.Progress)) (Runtime, error) {
	required := d.RequiredJava()
	compatible := d.CompatibleJavaMajors
	if len(compatible) == 0 {
		compatible = []int{required}
	}
	if custom != "" {
		j, err := inspectCached(ctx, custom)
		if err != nil {
			return Runtime{}, fmt.Errorf(locales.SelectedJavaIsUnavailable(), err)
		}
		if !ignoreCompatibility && !slices.Contains(compatible, j.Major) {
			return Runtime{}, fmt.Errorf(locales.ThisInstanceRequiresJavaButJavaIsSelectedEnableSkipJavaCompatibilityChecksToLaunchAnyway(), compatible, j.Major)
		}
		return j, nil
	}
	for _, j := range list {
		if err := ctx.Err(); err != nil {
			return Runtime{}, err
		}
		if !slices.Contains(compatible, j.Major) {
			continue
		}
		// A runtime may have changed or disappeared since the settings scan.
		if checked, err := inspectCached(ctx, j.Path); err == nil && slices.Contains(compatible, checked.Major) {
			return checked, nil
		}
	}
	for _, j := range Managed(ctx, dir) {
		if slices.Contains(compatible, j.Major) {
			return j, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return Runtime{}, err
	}
	if !auto {
		return Runtime{}, fmt.Errorf(locales.JavaNotFoundSelectItInSettingsOrEnableAutomaticDownload(), required)
	}
	component := ""
	if d.JavaVersion != nil {
		component = d.JavaVersion.Component
	}
	return Download(ctx, f, DownloadRequest{Distribution: DistributionMojang, PackageType: PackageJRE, Major: required, Component: component}, dir, progress)
}

func Download(ctx context.Context, f *download.Client, request DownloadRequest, dir string, progress func(minecraft.Progress)) (Runtime, error) {
	if request.Distribution == "" {
		request.Distribution = DistributionMojang
	}
	if request.PackageType == "" {
		request.PackageType = PackageJRE
	}
	release, err := f.Lock(ctx, "java", dir)
	if err != nil {
		return Runtime{}, err
	}
	defer release()
	if component := downloadComponent(request); component != "" {
		target := filepath.Join(dir, component, javaPlatform())
		for _, j := range Managed(ctx, dir) {
			relative, err := filepath.Rel(target, j.Path)
			if err == nil && j.Major == request.Major && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return j, nil
			}
		}
	}
	switch request.Distribution {
	case DistributionMojang:
		if request.PackageType != PackageJRE {
			return Runtime{}, fmt.Errorf(locales.AutomaticInstallationOfJavaIsNotSupported(), request.Major)
		}
		return downloadMojangRuntime(ctx, f, request, dir, progress)
	default:
		if _, ok := DistributionInfoFor(request.Distribution); !ok {
			return Runtime{}, fmt.Errorf(locales.AutomaticInstallationOfJavaIsNotSupported(), request.Major)
		}
		return downloadFoojayRuntime(ctx, f, request, dir, progress)
	}
}

func downloadComponent(request DownloadRequest) string {
	if request.Distribution != DistributionMojang {
		return fmt.Sprintf("%s-%s-%d", request.Distribution, request.PackageType, request.Major)
	}
	if request.Distribution == DistributionMojang && request.PackageType == PackageJRE {
		if request.Component != "" {
			return request.Component
		}
		return map[int]string{8: "jre-legacy", 16: "java-runtime-alpha", 17: "java-runtime-gamma", 21: "java-runtime-delta", 25: "java-runtime-epsilon"}[request.Major]
	}
	return ""
}

func downloadMojangRuntime(ctx context.Context, f *download.Client, request DownloadRequest, dir string, progress func(minecraft.Progress)) (Runtime, error) {
	required := request.Major
	component := request.Component
	if component == "" {
		component = map[int]string{8: "jre-legacy", 16: "java-runtime-alpha", 17: "java-runtime-gamma", 21: "java-runtime-delta", 25: "java-runtime-epsilon"}[required]
	}
	if component == "" {
		return Runtime{}, fmt.Errorf(locales.AutomaticInstallationOfJavaIsNotSupported(), required)
	}
	if err := fsutil.ValidateID(component); err != nil {
		return Runtime{}, err
	}
	platform := javaPlatform()
	if platform == "" {
		return Runtime{}, errors.New(locales.NoOfficialJavaRuntimeForThisPlatformChooseAPathManually())
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "java"})
	}
	var manifest map[string]map[string][]struct {
		Manifest minecraft.FileRef `json:"manifest"`
	}
	if err := f.GetJSON(ctx, runtimeManifestURL, &manifest); err != nil {
		return Runtime{}, err
	}
	entries := manifest[platform][component]
	if len(entries) == 0 {
		return Runtime{}, fmt.Errorf(locales.NoOfficialRuntimeForChooseJavaManually(), platform, component)
	}
	parent := filepath.Join(dir, component)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Runtime{}, err
	}
	root, err := os.MkdirTemp(parent, ".tomori-runtime-*")
	if err != nil {
		return Runtime{}, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	target := filepath.Join(parent, platform)
	ref := entries[0].Manifest
	manifestPath := filepath.Join(root, "manifest.json")
	if err := f.Fetch(ctx, download.Task{URL: ref.URL, Path: manifestPath, SHA1: ref.SHA1, Size: ref.Size}); err != nil {
		return Runtime{}, err
	}
	var files struct {
		Files map[string]runtimeFile `json:"files"`
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return Runtime{}, err
	}
	if err := json.Unmarshal(raw, &files); err != nil {
		return Runtime{}, err
	}
	if len(files.Files) == 0 {
		return Runtime{}, errors.New(locales.RuntimeFileListIsEmpty())
	}
	if len(files.Files) > 100000 {
		return Runtime{}, errors.New(locales.RuntimeFileListIsTooLarge())
	}
	var tasks []download.Task
	var total int64
	for name, entry := range files.Files {
		path, err := fsutil.SafePath(root, name)
		if err != nil {
			return Runtime{}, err
		}
		for part := range strings.SplitSeq(strings.ReplaceAll(name, "\\", "/"), "/") {
			if err := fsutil.ValidateID(part); err != nil {
				return Runtime{}, err
			}
		}
		switch entry.Type {
		case "file":
			ref := entry.Downloads.Raw
			if ref.URL == "" || len(ref.SHA1) != 40 || ref.Size <= 0 || ref.Size > 512<<20 || total > (4<<30)-ref.Size {
				return Runtime{}, fmt.Errorf(locales.RuntimeFileMetadataIncomplete(), name)
			}
			total += ref.Size
			tasks = append(tasks, download.Task{URL: ref.URL, Path: path, SHA1: ref.SHA1, Size: ref.Size})
		case "directory":
			if err := os.MkdirAll(path, 0o755); err != nil {
				return Runtime{}, err
			}
		case "link":
			if entry.Target == "" || filepath.IsAbs(entry.Target) || strings.ContainsAny(entry.Target, ":\\") {
				return Runtime{}, fmt.Errorf(locales.InvalidJavaLink(), name)
			}
			if _, err := fsutil.SafePath(root, filepath.Join(filepath.Dir(name), entry.Target)); err != nil {
				return Runtime{}, err
			}
		default:
			return Runtime{}, fmt.Errorf(locales.UnknownJavaFileTypeAlt(), entry.Type)
		}
	}
	if err := f.FetchAll(ctx, tasks, f.Workers, func(p download.Progress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "java", Done: p.Done, Total: p.Total, Files: p.Files, FilesDone: p.FilesDone, Download: &p})
		}
	}); err != nil {
		return Runtime{}, err
	}
	for name, entry := range files.Files {
		if err := ctx.Err(); err != nil {
			return Runtime{}, err
		}
		path, err := fsutil.SafePath(root, name)
		if err != nil {
			return Runtime{}, err
		}
		if entry.Type == "file" && entry.Executable {
			if err := os.Chmod(path, 0o755); err != nil {
				return Runtime{}, err
			}
		}
		if entry.Type != "link" {
			continue
		}
		if filepath.IsAbs(entry.Target) || strings.Contains(entry.Target, ":") {
			return Runtime{}, fmt.Errorf(locales.InvalidJavaLink(), name)
		}
		if _, err := fsutil.SafePath(root, filepath.Join(filepath.Dir(name), entry.Target)); err != nil {
			return Runtime{}, err
		}
		if target, err := os.Readlink(path); err == nil && target == entry.Target {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return Runtime{}, err
		}
		if err := os.Symlink(entry.Target, path); err != nil {
			return Runtime{}, err
		}
	}
	for _, path := range []string{filepath.Join(root, "bin", javaBinary()), filepath.Join(root, "jre.bundle", "Contents", "Home", "bin", javaBinary())} {
		if j, err := inspectCached(ctx, path); err == nil && j.Major == required {
			if err := fsutil.WriteAtomic(filepath.Join(root, ".complete"), []byte(component)); err != nil {
				return Runtime{}, err
			}
			if err := publishRuntime(ctx, root, target); err != nil {
				return Runtime{}, err
			}
			rel, err := filepath.Rel(root, j.Path)
			if err != nil {
				return Runtime{}, err
			}
			j.Path = filepath.Join(target, rel)
			return j, nil
		}
	}
	return Runtime{}, fmt.Errorf(locales.DownloadedJavaCannotRun(), required)
}

func publishRuntime(ctx context.Context, stage, target string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	backup := target + ".previous-" + rand.Text()
	if err := os.Rename(target, backup); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return os.Rename(stage, target)
	}
	if err := os.Rename(stage, target); err != nil {
		return errors.Join(err, os.Rename(backup, target))
	}
	_ = os.RemoveAll(backup)
	return nil
}

type runtimeFile struct {
	Type       string `json:"type"`
	Executable bool   `json:"executable"`
	Target     string `json:"target"`
	Downloads  struct {
		Raw minecraft.FileRef `json:"raw"`
	} `json:"downloads"`
}

func javaPlatform() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux"
	case "linux/386":
		return "linux-i386"
	case "windows/amd64":
		return "windows-x64"
	case "windows/386":
		return "windows-x86"
	case "windows/arm64":
		return "windows-arm64"
	case "darwin/amd64":
		return "mac-os"
	case "darwin/arm64":
		return "mac-os-arm64"
	}
	return ""
}
