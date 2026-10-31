package network

import (
	"context"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

func nativeProxy(ctx context.Context) httpproxy.Config {
	var cfg httpproxy.Config
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "gsettings", "list-recursively", "org.gnome.system.proxy").Output()
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
