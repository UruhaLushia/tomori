package proxy

import (
	"context"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/http/httpproxy"
)

func nativeProxy(ctx context.Context) httpproxy.Config {
	var cfg httpproxy.Config
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gsettings", "list-recursively", "org.gnome.system.proxy")
	cmd.Env = linuxSessionEnv()
	raw, err := cmd.Output()
	if err != nil {
		return cfg
	}
	values := make(map[string]string)
	for line := range strings.SplitSeq(string(raw), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) == 3 {
			values[strings.TrimPrefix(fields[0], "org.gnome.system.proxy")+"/"+fields[1]] = strings.Trim(fields[2], "'")
		}
	}
	if values["/mode"] != "manual" {
		return cfg
	}
	address := func(key, protocol string) string {
		return staticAddress(protocol, values[key+"/host"], values[key+"/port"])
	}
	cfg.HTTPProxy, cfg.HTTPSProxy = address(".http", "http"), address(".https", "http")
	if values[".http/use-authentication"] == "true" && cfg.HTTPProxy != "" {
		u, err := url.Parse(cfg.HTTPProxy)
		if err == nil {
			u.User = url.UserPassword(values[".http/authentication-user"], values[".http/authentication-password"])
			cfg.HTTPProxy = u.String()
		}
	}
	if values["/use-same-proxy"] == "true" {
		cfg.HTTPSProxy = cfg.HTTPProxy
	}
	socks := address(".socks", "socks5")
	if cfg.HTTPProxy == "" {
		cfg.HTTPProxy = socks
	}
	if cfg.HTTPSProxy == "" {
		cfg.HTTPSProxy = socks
	}
	for host := range strings.SplitSeq(strings.Trim(values["/ignore-hosts"], "[]"), ",") {
		host = strings.Trim(strings.TrimSpace(host), "'")
		if host != "" {
			cfg.NoProxy += host + ","
		}
	}
	return cfg
}

func linuxSessionEnv() []string {
	linuxSessionEnvState.Do(func() {
		linuxSessionEnvState.values = loadLinuxSessionEnv()
	})
	return linuxSessionEnvState.values
}

var linuxSessionEnvState struct {
	sync.Once
	values []string
}

func loadLinuxSessionEnv() []string {
	env := envMap(os.Environ())
	uid := uint32(os.Getuid())
	best, bestScore := env, sessionEnvScore(env)
	entries, err := os.ReadDir("/proc")
	if err == nil {
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil {
				continue
			}
			info, err := os.Stat(filepath.Join("/proc", entry.Name()))
			if err != nil {
				continue
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || stat.Uid != uid {
				continue
			}
			candidate, err := readProcessEnv(pid)
			if err != nil {
				continue
			}
			if score := sessionEnvScore(candidate); score > bestScore {
				best, bestScore = candidate, score
			}
		}
	}
	for key, value := range best {
		if env[key] == "" {
			env[key] = value
		}
	}
	if env["XDG_RUNTIME_DIR"] == "" {
		env["XDG_RUNTIME_DIR"] = filepath.Join("/run/user", strconv.FormatUint(uint64(uid), 10))
	}
	if env["DBUS_SESSION_BUS_ADDRESS"] == "" {
		env["DBUS_SESSION_BUS_ADDRESS"] = "unix:path=" + filepath.Join(env["XDG_RUNTIME_DIR"], "bus")
	}
	return envSlice(env)
}

func readProcessEnv(pid int) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return nil, err
	}
	return envMap(strings.Split(string(raw), "\x00")), nil
}

func envMap(values []string) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		key, item, ok := strings.Cut(value, "=")
		if ok {
			result[key] = item
		}
	}
	return result
}

func envSlice(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for key, value := range values {
		result = append(result, key+"="+value)
	}
	return result
}

func sessionEnvScore(env map[string]string) int {
	score := 0
	for key, points := range map[string]int{"XDG_CURRENT_DESKTOP": 8, "DBUS_SESSION_BUS_ADDRESS": 4, "XDG_RUNTIME_DIR": 2, "WAYLAND_DISPLAY": 1, "DISPLAY": 1, "HOME": 1} {
		if env[key] != "" {
			score += points
		}
	}
	return score
}
