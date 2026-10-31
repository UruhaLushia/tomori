package network

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/net/http/httpproxy"
)

func nativeProxy(ctx context.Context) httpproxy.Config {
	var cfg httpproxy.Config
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, "/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return cfg
	}
	values := make(map[string]string)
	var bypass []string
	exceptions := false
	for line := range strings.SplitSeq(string(raw), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), " : ")
		if ok && key == "ExceptionsList" {
			exceptions = true
			continue
		}
		if exceptions {
			if strings.TrimSpace(line) == "}" {
				exceptions = false
			} else if ok {
				bypass = append(bypass, value)
			}
			continue
		}
		if ok {
			values[key] = value
		}
	}
	if values["HTTPEnable"] == "1" {
		cfg.HTTPProxy = staticAddress("http", values["HTTPProxy"], values["HTTPPort"])
	}
	if values["HTTPSEnable"] == "1" {
		cfg.HTTPSProxy = staticAddress("http", values["HTTPSProxy"], values["HTTPSPort"])
	}
	if values["SOCKSEnable"] == "1" {
		socks := staticAddress("socks5", values["SOCKSProxy"], values["SOCKSPort"])
		if cfg.HTTPProxy == "" {
			cfg.HTTPProxy = socks
		}
		if cfg.HTTPSProxy == "" {
			cfg.HTTPSProxy = socks
		}
	}
	cfg.NoProxy = strings.Join(bypass, ",")
	return cfg
}
