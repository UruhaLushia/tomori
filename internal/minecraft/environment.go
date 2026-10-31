package minecraft

import "runtime"

type Environment struct {
	OS       string
	Arch     string
	Version  string
	Features map[string]bool
}

func CurrentEnvironment() Environment {
	env := Environment{OS: runtime.GOOS, Version: operatingSystemVersion(), Features: map[string]bool{}}
	switch env.OS {
	case "darwin":
		env.OS = "osx"
	case "windows", "linux":
	default:
		env.OS = "linux"
	}
	switch runtime.GOARCH {
	case "amd64":
		env.Arch = "x86_64"
	case "386":
		env.Arch = "x86"
	case "arm64":
		env.Arch = "arm64"
	case "arm":
		env.Arch = "arm32"
	default:
		env.Arch = runtime.GOARCH
	}
	return env
}
