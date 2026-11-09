package serverstatus

import (
	"errors"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/net/idna"

	"github.com/UruhaLushia/tomori/locales"
)

type Address struct {
	Host string
	Port uint16
}

func ParseAddress(value string) (Address, error) {
	value = strings.TrimSpace(value)
	invalid := errors.New(locales.EnterAValidServerAddress())
	if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "/\\@?#\x00") || strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsFunc(value, unicode.IsControl) {
		return Address{}, invalid
	}
	host, port := value, "25565"
	if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
		host = value[1 : len(value)-1]
	} else if strings.HasPrefix(value, "[") || strings.Count(value, ":") == 1 {
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil {
			return Address{}, invalid
		}
	}
	if strings.HasPrefix(value, "[") {
		if ip, err := netip.ParseAddr(host); err != nil || !ip.Is6() {
			return Address{}, invalid
		}
	}
	if _, err := netip.ParseAddr(host); err != nil {
		if strings.ContainsAny(host, ":[]%") {
			return Address{}, invalid
		}
		host, err = idna.Lookup.ToASCII(host)
		if err != nil || host == "" || host == "." || strings.HasPrefix(host, "-") || strings.Contains(host, "..") {
			return Address{}, invalid
		}
	}
	number, err := strconv.ParseUint(port, 10, 16)
	if err != nil || number == 0 || len(host) > 255 {
		return Address{}, invalid
	}
	return Address{Host: host, Port: uint16(number)}, nil
}

func (a Address) String() string { return net.JoinHostPort(a.Host, strconv.Itoa(int(a.Port))) }
