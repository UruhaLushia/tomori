package java

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type Runtime struct {
	Path    string
	Version string
	Major   int
	Vendor  string
	Arch    string
}

func (j Runtime) Label() string {
	if j.Vendor != "" {
		return fmt.Sprintf("Java %s · %s", j.Version, j.Vendor)
	}
	return "Java " + j.Version
}

type probeCacheEntry struct {
	size    int64
	modTime int64
	runtime Runtime
	err     error
}

var probeCache = struct {
	sync.Mutex
	entries map[string]probeCacheEntry
}{entries: make(map[string]probeCacheEntry)}

func ExecutableNames() []string {
	if runtime.GOOS == "windows" {
		return []string{"java.exe", "javaw.exe"}
	}
	return []string{"java"}
}

// InspectExecutable validates a file picked by the user before executing it.
func InspectExecutable(ctx context.Context, path string) (Runtime, error) {
	path, err := fsutil.DirectoryPath(path)
	if err != nil {
		return Runtime{}, err
	}
	name := filepath.Base(path)
	if runtime.GOOS == "windows" {
		name = strings.ToLower(name)
	}
	valid := slices.Contains(ExecutableNames(), name)
	if !valid {
		return Runtime{}, errors.New(locales.SelectAJavaExecutable())
	}
	info, err := os.Stat(path)
	if err != nil {
		return Runtime{}, err
	}
	if !info.Mode().IsRegular() {
		return Runtime{}, errors.New(locales.SelectAJavaExecutable())
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return Runtime{}, errors.New(locales.FileIsNotExecutable())
	}
	j, err := inspectCached(ctx, path)
	if err != nil {
		return Runtime{}, fmt.Errorf(locales.CannotReadJavaVersion(), err)
	}
	if j.Major <= 0 {
		return Runtime{}, errors.New(locales.UnrecognizedJavaVersion())
	}
	return j, nil
}

func Discover(ctx context.Context, custom string, manual ...string) []Runtime {
	var found []Runtime
	for _, path := range javaCandidates(custom, manual...) {
		if ctx.Err() != nil {
			break
		}
		info, err := inspectCached(ctx, path)
		if err != nil {
			continue
		}
		found = append(found, info)
	}
	sort.SliceStable(found, func(a, b int) bool { return found[a].Major > found[b].Major })
	return found
}

func Inspect(parent context.Context, path string) (Runtime, error) {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()
	if info, ok := readRelease(path); ok {
		info.Path = path
		return info, nil
	}
	probePath := path
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Base(path), "javaw.exe") {
		candidate := filepath.Join(filepath.Dir(path), "java.exe")
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			probePath = candidate
		}
	}
	cmd := exec.CommandContext(ctx, probePath, "-XshowSettings:properties", "-version")
	prepareCommand(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Runtime{}, err
	}
	text := string(out)
	info := Runtime{Path: path}
	info.Version, info.Vendor, info.Arch = parseOutput(text)
	if info.Version == "" {
		return Runtime{}, errors.New(locales.UnrecognizedJavaVersion())
	}
	info.Major = majorOf(info.Version)
	return info, nil
}

func inspectCached(ctx context.Context, path string) (Runtime, error) {
	info, statErr := os.Stat(path)
	if statErr != nil || !info.Mode().IsRegular() {
		return Inspect(ctx, path)
	}
	key := probeKey(path)
	modTime := info.ModTime().UnixNano()
	probeCache.Lock()
	entry, ok := probeCache.entries[key]
	probeCache.Unlock()
	if ok && entry.size == info.Size() && entry.modTime == modTime {
		entry.runtime.Path = path
		return entry.runtime, entry.err
	}
	runtimeInfo, err := Inspect(ctx, path)
	if ctx.Err() != nil {
		return runtimeInfo, err
	}
	probeCache.Lock()
	probeCache.entries[key] = probeCacheEntry{size: info.Size(), modTime: modTime, runtime: runtimeInfo, err: err}
	probeCache.Unlock()
	return runtimeInfo, err
}

func readRelease(path string) (Runtime, bool) {
	original := path
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	root := filepath.Dir(filepath.Dir(path))
	release, err := os.Open(filepath.Join(root, "release"))
	if err != nil {
		return Runtime{}, false
	}
	defer func() { _ = release.Close() }()
	var version, vendor, arch string
	scanner := bufio.NewScanner(release)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		switch key {
		case "JAVA_VERSION":
			version = value
		case "IMPLEMENTOR":
			vendor = value
		case "OS_ARCH":
			arch = value
		}
		if version != "" && vendor != "" && arch != "" {
			break
		}
	}
	if scanner.Err() != nil || version == "" || majorOf(version) <= 0 {
		return Runtime{}, false
	}
	return Runtime{Path: original, Version: version, Major: majorOf(version), Vendor: vendor, Arch: arch}, true
}

func probeKey(path string) string {
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = real
	}
	path, _ = filepath.Abs(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return filepath.Clean(path)
}

func parseOutput(text string) (version, vendor, arch string) {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if value, ok := propertyValue(line, "java.version"); ok {
			version = value
			continue
		}
		if value, ok := propertyValue(line, "java.vendor"); ok {
			vendor = value
			continue
		}
		if value, ok := propertyValue(line, "os.arch"); ok {
			arch = value
			continue
		}
		if version == "" {
			const marker = "version \""
			if start := strings.Index(line, marker); start >= 0 {
				value := line[start+len(marker):]
				if before, _, ok := strings.Cut(value, "\""); ok {
					version = before
				}
			}
		}
	}
	return version, vendor, arch
}

func propertyValue(line, key string) (string, bool) {
	if !strings.HasPrefix(line, key) {
		return "", false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, key))
	if !strings.HasPrefix(rest, "=") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "=")), true
}

func majorOf(version string) int {
	parts := strings.FieldsFunc(version, func(r rune) bool { return r == '.' || r == '_' || r == '+' || r == '-' })
	if len(parts) == 0 {
		return 0
	}
	if parts[0] == "1" && len(parts) > 1 {
		n, _ := strconv.Atoi(parts[1])
		return n
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}

func javaCandidates(custom string, manual ...string) []string {
	var list []string
	seen := make(map[string]bool)
	add := func(path string) {
		if path == "" {
			return
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return
		}
		key := probeKey(path)
		if seen[key] {
			return
		}
		seen[key] = true
		list = append(list, path)
	}
	if custom != "" {
		add(custom)
	}
	for _, path := range manual {
		add(path)
	}
	if home := os.Getenv("JAVA_HOME"); home != "" {
		add(filepath.Join(home, "bin", javaBinary()))
	}
	if p, err := exec.LookPath(javaBinary()); err == nil {
		add(p)
	}
	home, _ := os.UserHomeDir()
	var patterns []string
	switch runtime.GOOS {
	case "windows":
		for _, base := range []string{
			os.Getenv("ProgramFiles"),
			os.Getenv("ProgramFiles(x86)"),
			os.Getenv("LOCALAPPDATA") + `\Programs`,
		} {
			if base == "" {
				continue
			}
			patterns = append(patterns,
				filepath.Join(base, `Java`, `*`, `bin`, `java.exe`),
				filepath.Join(base, `Eclipse Adoptium`, `*`, `bin`, `java.exe`),
				filepath.Join(base, `Microsoft`, `jdk*`, `bin`, `java.exe`),
				filepath.Join(base, `Zulu`, `*`, `bin`, `java.exe`),
				filepath.Join(base, `BellSoft`, `*`, `bin`, `java.exe`),
				filepath.Join(base, `Amazon Corretto`, `*`, `bin`, `java.exe`),
			)
		}
	case "darwin":
		for _, base := range []string{
			"/Library/Java/JavaVirtualMachines",
			"/System/Library/Java/JavaVirtualMachines",
			filepath.Join(home, "Library/Java/JavaVirtualMachines"),
		} {
			patterns = append(patterns, filepath.Join(base, "*", "Contents", "Home", "bin", "java"))
		}
	default:
		for _, base := range []string{
			"/usr/lib/jvm",
			"/usr/java",
			"/opt/java",
			filepath.Join(home, ".jdks"),
			filepath.Join(home, ".sdkman/candidates/java"),
			filepath.Join(home, ".local/share/jdks"),
		} {
			patterns = append(patterns,
				filepath.Join(base, "*", "bin", "java"),
				filepath.Join(base, "*", "jre", "bin", "java"),
			)
		}
	}
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			continue
		}
		for _, match := range matches {
			add(match)
		}
	}
	return list
}

func javaBinary() string {
	if runtime.GOOS == "windows" {
		return "java.exe"
	}
	return "java"
}
