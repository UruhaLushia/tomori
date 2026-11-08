package loader

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/locales"
)

type Version struct {
	ID     string
	Stable bool
}

func Versions(ctx context.Context, client *httpclient.Client, game, kind string) ([]Version, error) {
	if err := fsutil.ValidateID(game); err != nil {
		return nil, err
	}
	var versions []Version
	switch kind {
	case "fabric", "quilt":
		host, api := "https://meta.fabricmc.net", "v2"
		if kind == "quilt" {
			host, api = "https://meta.quiltmc.org", "v3"
		}
		var entries []struct {
			Loader struct {
				Version string `json:"version"`
				Stable  bool   `json:"stable"`
			} `json:"loader"`
		}
		if err := client.GetJSON(ctx, host+"/"+api+"/versions/loader/"+url.PathEscape(game), &entries); err != nil {
			return nil, err
		}
		for _, entry := range entries {
			stable := entry.Loader.Stable
			if kind == "quilt" {
				stable = stableVersion(entry.Loader.Version)
			}
			versions = append(versions, Version{ID: entry.Loader.Version, Stable: stable})
		}
	case "forge":
		var entries map[string][]string
		if err := client.GetJSON(ctx, "https://files.minecraftforge.net/net/minecraftforge/forge/maven-metadata.json", &entries); err != nil {
			return nil, err
		}
		for _, version := range entries[game] {
			versions = append(versions, Version{ID: strings.TrimPrefix(version, game+"-"), Stable: stableVersion(version)})
		}
	case "neoforge":
		artifact, prefix := "neoforge", neoForgePrefix(game)
		if game == "1.20.1" {
			artifact, prefix = "forge", game+"-"
		}
		if prefix == "" {
			return nil, nil
		}
		raw, err := client.Get(ctx, "https://maven.neoforged.net/releases/net/neoforged/"+artifact+"/maven-metadata.xml")
		if err != nil {
			return nil, err
		}
		var metadata struct {
			Versions []string `xml:"versioning>versions>version"`
		}
		if err := xml.Unmarshal(raw, &metadata); err != nil {
			return nil, err
		}
		for _, version := range metadata.Versions {
			if strings.HasPrefix(version, prefix) {
				versions = append(versions, Version{ID: strings.TrimPrefix(version, game+"-"), Stable: stableVersion(version)})
			}
		}
	default:
		return nil, fmt.Errorf(locales.InvalidLoader(), kind)
	}
	versions = slices.DeleteFunc(versions, func(v Version) bool { return fsutil.ValidateID(v.ID) != nil })
	slices.SortStableFunc(versions, func(a, b Version) int {
		if a.Stable != b.Stable {
			if a.Stable {
				return -1
			}
			return 1
		}
		return compareVersion(b.ID, a.ID)
	})
	return slices.CompactFunc(versions, func(a, b Version) bool { return a.ID == b.ID }), nil
}

func stableVersion(v string) bool {
	v = strings.ToLower(v)
	return !strings.Contains(v, "alpha") && !strings.Contains(v, "beta") && !strings.Contains(v, "snapshot") && !strings.Contains(v, "rc")
}

func neoForgePrefix(game string) string {
	parts := strings.Split(game, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return ""
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return ""
		}
	}
	if parts[0] == "1" {
		minor, _ := strconv.Atoi(parts[1])
		if minor < 20 {
			return ""
		}
		patch := "0"
		if len(parts) == 3 {
			patch = parts[2]
		}
		return parts[1] + "." + patch + "."
	}
	major, _ := strconv.Atoi(parts[0])
	if major < 26 {
		return ""
	}
	if len(parts) == 2 {
		return game + ".0."
	}
	return game + "."
}

func compareVersion(a, b string) int {
	for a != "" && b != "" {
		x, y := versionPart(&a), versionPart(&b)
		if decimalPart(x) && decimalPart(y) {
			x, y = strings.TrimLeft(x, "0"), strings.TrimLeft(y, "0")
			if len(x) < len(y) {
				return -1
			}
			if len(x) > len(y) {
				return 1
			}
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	if a != "" {
		return 1
	}
	if b != "" {
		return -1
	}
	return 0
}

func versionPart(v *string) string {
	part := *v
	if end := strings.IndexAny(part, ".-+"); end >= 0 {
		*v = strings.TrimLeft(part[end+1:], ".-+")
		return part[:end]
	}
	*v = ""
	return part
}

func decimalPart(v string) bool {
	if v == "" {
		return false
	}
	for i := range len(v) {
		if v[i] < '0' || v[i] > '9' {
			return false
		}
	}
	return true
}
