package network

import (
	"context"
	"strings"

	"golang.org/x/net/http/httpproxy"
	"golang.org/x/sys/windows/registry"
)

func nativeProxy(context.Context) httpproxy.Config {
	var cfg httpproxy.Config
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Internet Settings`, registry.QUERY_VALUE)
	if err != nil {
		return cfg
	}
	defer k.Close()
	enabled, _, _ := k.GetIntegerValue("ProxyEnable")
	if enabled == 0 {
		return cfg
	}
	server, _, _ := k.GetStringValue("ProxyServer")
	bypass, _, _ := k.GetStringValue("ProxyOverride")
	cfg.NoProxy = strings.ReplaceAll(bypass, ";", ",")
	if !strings.Contains(server, "=") {
		if server != "" && !strings.Contains(server, "://") {
			server = "http://" + server
		}
		cfg.HTTPProxy, cfg.HTTPSProxy = server, server
		return cfg
	}
	var socks string
	for part := range strings.SplitSeq(server, ";") {
		protocol, address, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok || address == "" {
			continue
		}
		if !strings.Contains(address, "://") {
			scheme := "http"
			if protocol == "socks" {
				scheme = "socks5"
			}
			address = scheme + "://" + address
		}
		switch protocol {
		case "http":
			cfg.HTTPProxy = address
		case "https":
			cfg.HTTPSProxy = address
		case "socks":
			socks = address
		}
	}
	if cfg.HTTPProxy == "" {
		cfg.HTTPProxy = socks
	}
	if cfg.HTTPSProxy == "" {
		cfg.HTTPSProxy = socks
	}
	return cfg
}
