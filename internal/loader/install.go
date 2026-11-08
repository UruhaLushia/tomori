package loader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/locales"
)

type Installer struct {
	Fetcher           *download.Client
	Root, Cache, Java string
	ReuseRoots        []string
	Progress          func(minecraft.Progress)
	OnLine            func(string)
}

func (i *Installer) Install(ctx context.Context, base *minecraft.VersionDetail, kind modpack.Loader) (*minecraft.VersionDetail, error) {
	if err := fsutil.ValidateID(kind.Version); err != nil {
		return nil, err
	}
	if kind.ID == "fabric" || kind.ID == "quilt" {
		host, api := "https://meta.fabricmc.net", "v2"
		if kind.ID == "quilt" {
			host, api = "https://meta.quiltmc.org", "v3"
		}
		var profile minecraft.VersionDetail
		address := fmt.Sprintf("%s/%s/versions/loader/%s/%s/profile/json", host, api, url.PathEscape(base.ID), url.PathEscape(kind.Version))
		if err := i.Fetcher.GetJSON(ctx, address, &profile); err != nil {
			return nil, err
		}
		if profile.InheritsFrom != "" && profile.InheritsFrom != base.ID {
			return nil, errors.New(locales.LoaderGameVersionMismatch())
		}
		v := minecraft.MergeVersion(base, &profile)
		if v.MainClass == base.MainClass {
			return nil, errors.New(locales.LoaderHasNoMainClass())
		}
		return v, nil
	}
	if kind.ID != "forge" && kind.ID != "neoforge" {
		return nil, fmt.Errorf(locales.UnsupportedLoader2(), kind.String())
	}
	if i.Java == "" {
		return nil, fmt.Errorf(locales.JavaIsRequiredToInstall(), kind.String())
	}
	version := kind.Version
	group, artifact, repo := "net.minecraftforge", "forge", "https://maven.minecraftforge.net"
	if kind.ID == "forge" {
		if !strings.HasPrefix(version, base.ID+"-") {
			version = base.ID + "-" + version
		}
	} else {
		group, artifact, repo = "net.neoforged", "neoforge", "https://maven.neoforged.net/releases"
		if base.ID == "1.20.1" {
			artifact = "forge"
			if !strings.HasPrefix(version, base.ID+"-") {
				version = base.ID + "-" + version
			}
		}
	}
	coordinate := group + ":" + artifact + ":" + version + ":installer"
	maven, err := minecraft.MavenPath(coordinate)
	if err != nil {
		return nil, err
	}
	filename := cache.FileIn(i.Cache, "artifacts", maven)
	var checksum string
	if raw, err := i.Fetcher.Get(ctx, repo+"/"+maven+".sha1"); err == nil {
		checksum = strings.TrimSpace(string(raw))
		if fields := strings.Fields(checksum); len(fields) > 0 {
			checksum = fields[0]
		}
	}
	if len(checksum) != 40 {
		return nil, fmt.Errorf(locales.CannotRetrieveChecksumForInstaller(), kind.String())
	}
	if err := i.Fetcher.FetchAll(ctx, []download.Task{{URL: repo + "/" + maven, Path: filename, SHA1: checksum}}, 1, i.downloadProgress); err != nil {
		return nil, err
	}
	return i.installForge(ctx, base, filename)
}

func (i *Installer) downloadProgress(p download.Progress) {
	if i.Progress != nil {
		i.Progress(minecraft.Progress{Stage: "loader", Download: &p, Done: p.Done, Total: p.Total, Files: p.Files, FilesDone: p.FilesDone})
	}
}
