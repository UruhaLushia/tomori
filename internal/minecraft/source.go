package minecraft

import (
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

type Source string

const (
	SourceOfficial Source = "official"
	SourceMirror   Source = "bmclapi"
)

func ParseSource(s string) Source {
	if s == string(SourceMirror) {
		return SourceMirror
	}
	return SourceOfficial
}

func (s Source) Name() string {
	if s == SourceMirror {
		return locales.BmclapiMirror()
	}
	return locales.MojangOfficialSource()
}

var mirrorHosts = []struct{ host, prefix string }{
	{"piston-meta.mojang.com", ""},
	{"launchermeta.mojang.com", ""},
	{"launcher.mojang.com", ""},
	{"piston-data.mojang.com", ""},
	{"libraries.minecraft.net", "/maven"},
	{"maven.minecraftforge.net", "/maven"},
	{"maven.fabricmc.net", "/maven"},
	{"meta.fabricmc.net", "/fabric-meta"},
	{"resources.download.minecraft.net", "/assets"},
}

func (s Source) Rewrite(url string) string {
	if s != SourceMirror || url == "" {
		return url
	}
	rest, ok := strings.CutPrefix(url, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(url, "http://")
		if !ok {
			return url
		}
	}
	host, path, _ := strings.Cut(rest, "/")
	for _, m := range mirrorHosts {
		if host == m.host {
			if path == "" {
				return "https://bmclapi2.bangbang93.com" + m.prefix
			}
			return "https://bmclapi2.bangbang93.com" + m.prefix + "/" + path
		}
	}
	return url
}
