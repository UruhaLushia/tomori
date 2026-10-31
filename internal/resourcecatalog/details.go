package resourcecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"github.com/UruhaLushia/tomori/internal/modrinth"
	"github.com/UruhaLushia/tomori/locales"
)

type VersionDetails struct {
	Changelog             string
	GameVersions, Loaders []string
	Truncated             bool
}

func (c Client) Details(ctx context.Context, version Version) (VersionDetails, error) {
	if !version.Source.Valid() || version.ID == "" || version.ProjectID == "" {
		return VersionDetails{}, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
	}
	var result VersionDetails
	if version.Source == CurseForge {
		file, err := c.curseFile(ctx, version.ProjectID, version.ID)
		if err != nil {
			return result, err
		}
		result.GameVersions = file.GameVersions
		body, err := c.Fetcher.GetLimited(ctx, curseAPI+"/mods/"+version.ProjectID+"/files/"+version.ID+"/changelog", 2<<20)
		if err != nil {
			return result, err
		}
		var data struct{ Data string }
		if err := json.Unmarshal(body, &data); err != nil {
			return result, err
		}
		result.Changelog = plainHTML(data.Data)
	} else {
		body, err := c.Fetcher.GetLimited(ctx, "https://api.modrinth.com/v2/version/"+url.PathEscape(version.ID), 2<<20)
		if err != nil {
			return result, err
		}
		var data struct {
			modrinth.Version
			Changelog string
		}
		if err := json.Unmarshal(body, &data); err != nil {
			return result, err
		}
		if data.ID != version.ID || data.ProjectID != version.ProjectID {
			return result, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
		}
		result.GameVersions, result.Loaders, result.Changelog = data.GameVersions, data.Loaders, data.Changelog
	}
	result.Changelog = strings.TrimSpace(result.Changelog)
	if len(result.Changelog) > 32<<10 {
		end := 32 << 10
		for end > 0 && !utf8.RuneStart(result.Changelog[end]) {
			end--
		}
		result.Changelog, result.Truncated = strings.Clone(result.Changelog[:end]), true
	}
	return result, ctx.Err()
}

func plainHTML(source string) string {
	z := html.NewTokenizer(strings.NewReader(source))
	z.SetMaxBuf(2 << 20)
	var out strings.Builder
	ignored := ""
	for out.Len() <= 32<<10 {
		token := z.Next()
		if token == html.ErrorToken {
			if z.Err() != io.EOF {
				return ""
			}
			break
		}
		if token == html.TextToken && ignored == "" {
			out.Write(z.Text())
			continue
		}
		if token != html.StartTagToken && token != html.EndTagToken && token != html.SelfClosingTagToken {
			continue
		}
		name, _ := z.TagName()
		tag := string(name)
		if token == html.EndTagToken && tag == ignored {
			ignored = ""
			continue
		}
		if ignored != "" {
			continue
		}
		if tag == "script" || tag == "style" {
			if token == html.StartTagToken {
				ignored = tag
			}
			continue
		}
		if tag == "br" || token == html.EndTagToken && (tag == "p" || tag == "div" || tag == "li" || tag == "h1" || tag == "h2" || tag == "h3" || tag == "pre") {
			out.WriteByte('\n')
		}
	}
	return out.String()
}
