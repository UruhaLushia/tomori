package account

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

func (c *Client) externalTexture(ctx context.Context, a Account, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || len(address) > 4096 {
		return nil, errors.New(locales.InvalidSkinUrl())
	}
	host := strings.ToLower(u.Hostname())
	allowed := matchTextureDomain(".minecraft.net", host) || matchTextureDomain(".mojang.com", host)
	if !allowed {
		server, err := c.YggdrasilServer(ctx, a.ServerURL, false)
		if err != nil {
			return nil, err
		}
		var info struct {
			SkinDomains []string `json:"skinDomains"`
		}
		if json.Unmarshal([]byte(server.Metadata), &info) != nil || len(info.SkinDomains) > 1024 {
			return nil, errors.New(locales.InvalidAuthenticationServerResponse())
		}
		for _, domain := range info.SkinDomains {
			if matchTextureDomain(strings.ToLower(domain), host) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return nil, errors.New(locales.SkinUrlIsOutsideTheServerSAllowedDomains())
	}
	response, release, err := c.publicAuthGet(ctx, address, false)
	if err != nil {
		return nil, err
	}
	defer release()
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, skinLimit+1))
	if err != nil {
		return nil, err
	}
	if _, err := skinConfig(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func matchTextureDomain(pattern, host string) bool {
	if len(pattern) > 1024 {
		return false
	}
	part, rest, wild := strings.Cut(pattern, "*")
	if !wild {
		if strings.HasPrefix(pattern, ".") {
			return strings.HasSuffix(host, pattern)
		}
		return host == pattern
	}
	if !strings.HasPrefix(host, part) {
		return false
	}
	host = host[len(part):]
	for {
		part, rest, wild = strings.Cut(rest, "*")
		if !wild {
			return strings.HasSuffix(host, part)
		}
		index := strings.Index(host, part)
		if index < 0 {
			return false
		}
		host = host[index+len(part):]
	}
}
