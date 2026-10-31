package minecraft

import (
	"regexp"
	"strings"
)

type Rule struct {
	Action   string          `json:"action"`
	OS       *OSRule         `json:"os"`
	Features map[string]bool `json:"features"`
}

type OSRule struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

func Allowed(rules []Rule, env Environment) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if r.matches(env) {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}

func (r Rule) matches(env Environment) bool {
	if r.OS != nil {
		if r.OS.Name != "" && !matchOS(r.OS.Name, env.OS) {
			return false
		}
		if r.OS.Arch != "" && !matchArch(r.OS.Arch, env.Arch) {
			return false
		}
		if r.OS.Version != "" && !matchVersion(r.OS.Version, env.Version) {
			return false
		}
	}
	for name, want := range r.Features {
		if env.Features[name] != want {
			return false
		}
	}
	return true
}

func matchOS(rule, env string) bool {
	if rule == env {
		return true
	}
	return (rule == "osx" && env == "darwin") || (rule == "darwin" && env == "osx")
}

func matchArch(rule, env string) bool {
	if rule == env {
		return true
	}
	switch rule {
	case "x86_64", "amd64":
		return env == "x86_64" || env == "amd64"
	case "x86", "i386", "386":
		return env == "x86" || env == "386" || env == "i386"
	case "arm64", "aarch64":
		return env == "arm64" || env == "aarch64"
	case "arm32", "arm":
		return env == "arm32" || env == "arm"
	}
	return false
}

func matchVersion(pattern, version string) bool {
	if version == "" {
		return false
	}
	ok, err := regexp.MatchString(pattern, version)
	return err == nil && ok
}

func NativeClassifier(natives map[string]string, env Environment) string {
	// Modern metadata may provide an exact [os]-[arch] classifier. Mojang's
	// legacy [os] fallback is intentionally limited to x86/x86_64, matching
	// Prism's RuntimeContext behavior so ARM never receives an x86 native jar.
	name, arch := env.OS, nativeArch(env.Arch)
	if name == "darwin" {
		name = "osx"
	}
	value := natives[name+"-"+arch]
	if value == "" && (arch == "x86" || arch == "x86_64") {
		value = natives[name]
	}
	return strings.ReplaceAll(value, "${arch}", archBits(arch))
}

func nativeArch(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "386", "i386":
		return "x86"
	case "aarch64":
		return "arm64"
	case "arm":
		return "arm32"
	default:
		return arch
	}
}

func archBits(arch string) string {
	if strings.Contains(arch, "64") {
		return "64"
	}
	return "32"
}

func (l Library) Applies(env Environment) bool {
	return Allowed(l.Rules, env)
}
