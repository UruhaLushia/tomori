package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/net/http/httpproxy"

	"github.com/UruhaLushia/tomori/locales"
)

const (
	System = "system"
	Direct = "direct"
	Manual = "manual"
)

type Proxy struct {
	Mode     string `json:"mode"`
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

func DefaultProxy() Proxy {
	return Proxy{Mode: System, Protocol: "http", Host: "127.0.0.1", Port: 7890}
}

func (p Proxy) Validate() error {
	switch p.Mode {
	case "", System, Direct:
		return nil
	case Manual:
		_, err := p.proxyURL()
		return err
	default:
		return errors.New(locales.InvalidProxyMode())
	}
}

func (p Proxy) proxyURL() (*url.URL, error) {
	switch p.Protocol {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, errors.New(locales.InvalidProxyProtocol())
	}
	host := strings.TrimSpace(p.Host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	if host == "" || strings.ContainsAny(host, "/@?#\\[]") || strings.ContainsFunc(host, unicode.IsSpace) ||
		(strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return nil, errors.New(locales.EnterProxyHostnameOrIp())
	}
	if p.Port < 1 || p.Port > 65535 {
		return nil, errors.New(locales.ProxyPortMustBeBetween1And65535())
	}
	if p.Username == "" && p.Password != "" {
		return nil, errors.New(locales.EnterProxyUsername())
	}
	u := &url.URL{Scheme: p.Protocol, Host: net.JoinHostPort(host, strconv.Itoa(p.Port))}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return u, nil
}

// Transport belongs to one job. Proxy settings and connection pools are never
// changed under an in-flight request, nor written into process environment.
func Transport(p Proxy) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if err := p.Validate(); err != nil {
		t.Proxy = func(*http.Request) (*url.URL, error) { return nil, err }
		return t
	}
	switch p.Mode {
	case Direct:
		t.Proxy = nil
	case Manual:
		u, _ := p.proxyURL()
		t.Proxy = http.ProxyURL(u)
	default:
		var once sync.Once
		var resolve func(*url.URL) (*url.URL, error)
		t.Proxy = func(req *http.Request) (*url.URL, error) {
			once.Do(func() {
				cfg := systemProxy(req.Context())
				local := false
				for host := range strings.SplitSeq(cfg.NoProxy, ",") {
					if strings.TrimSpace(host) == "<local>" {
						local = true
					}
				}
				cfg.NoProxy = normalizeBypass(cfg.NoProxy)
				proxy := cfg.ProxyFunc()
				resolve = func(u *url.URL) (*url.URL, error) {
					if local && !strings.ContainsAny(u.Hostname(), ".:") {
						return nil, nil
					}
					return proxy(u)
				}
			})
			return resolve(req.URL)
		}
	}
	return t
}

// Desktop proxy settings also accept IPv4 wildcards, unlike NO_PROXY.
func normalizeBypass(value string) string {
	hosts := strings.Split(value, ",")
	for i, host := range hosts {
		host = strings.TrimSpace(host)
		parts := strings.Split(host, ".")
		if len(parts) < 2 || len(parts) > 4 || parts[len(parts)-1] != "*" {
			continue
		}
		var octets [4]byte
		valid := true
		for j, part := range parts[:len(parts)-1] {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n > 255 {
				valid = false
				break
			}
			octets[j] = byte(n)
		}
		if valid {
			hosts[i] = fmt.Sprintf("%s/%d", net.IPv4(octets[0], octets[1], octets[2], octets[3]), (len(parts)-1)*8)
		}
	}
	return strings.Join(hosts, ",")
}

func systemProxy(ctx context.Context) *httpproxy.Config {
	cfg := httpproxy.FromEnvironment()
	all := os.Getenv("ALL_PROXY")
	if all == "" {
		all = os.Getenv("all_proxy")
	}
	if cfg.HTTPProxy == "" {
		cfg.HTTPProxy = all
	}
	if cfg.HTTPSProxy == "" {
		cfg.HTTPSProxy = all
	}
	if cfg.HTTPProxy != "" && cfg.HTTPSProxy != "" {
		return cfg
	}
	native := nativeProxy(ctx)
	if cfg.HTTPProxy == "" {
		cfg.HTTPProxy = native.HTTPProxy
	}
	if cfg.HTTPSProxy == "" {
		cfg.HTTPSProxy = native.HTTPSProxy
	}
	if native.NoProxy != "" {
		cfg.NoProxy = strings.Trim(cfg.NoProxy+","+native.NoProxy, ",")
	}
	return cfg
}

func staticAddress(protocol, host, port string) string {
	if host == "" || port == "" || port == "0" {
		return ""
	}
	return (&url.URL{Scheme: protocol, Host: net.JoinHostPort(host, port)}).String()
}
