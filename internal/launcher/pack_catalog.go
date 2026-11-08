package launcher

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) PackSearch(ctx context.Context, filter packcatalog.Filter) (packcatalog.Result, error) {
	return (packcatalog.Client{Fetcher: s.http}).Search(ctx, filter)
}
func (s *Service) PackCategories(ctx context.Context, source string) ([]packcatalog.Category, error) {
	return (packcatalog.Client{Fetcher: s.http}).Categories(ctx, source)
}
func (s *Service) PackVersions(ctx context.Context, source, project, game string) ([]packcatalog.Version, error) {
	return (packcatalog.Client{Fetcher: s.http}).Versions(ctx, source, project, game)
}
func (s *Service) ResolvePackURL(ctx context.Context, raw string) (packcatalog.Version, error) {
	return (packcatalog.Client{Fetcher: s.http}).Resolve(ctx, raw)
}

func (s *Service) PackOrigin(ctx context.Context, instance minecraft.Instance) (modpack.OriginHeader, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return modpack.OriginHeader{}, err
	}
	defer release()
	root, err := minecraft.OpenInstanceRoot(ctx, instance)
	if err != nil {
		return modpack.OriginHeader{}, err
	}
	defer root.Close()
	header, err := modpack.ReadOriginHeader(ctx, root)
	if errors.Is(err, os.ErrNotExist) {
		return header, errors.New(locales.MissingModpackOriginRecord())
	}
	return header, err
}

func (s *Service) InspectPack(ctx context.Context, path string) (modpack.Info, modpack.ArchiveIdentity, error) {
	archive, identity, err := s.openPack(ctx, path)
	if err != nil {
		return modpack.Info{}, modpack.ArchiveIdentity{}, err
	}
	defer archive.Close()
	return archive.Info, identity, nil
}

func (s *Service) openPack(ctx context.Context, path string) (*modpack.Archive, modpack.ArchiveIdentity, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, modpack.ArchiveIdentity{}, err
	}
	defer release()
	archive, err := modpack.Open(ctx, path)
	if err != nil {
		return nil, modpack.ArchiveIdentity{}, err
	}
	identity, err := archive.Identity(ctx)
	if err != nil {
		_ = archive.Close()
		return nil, modpack.ArchiveIdentity{}, err
	}
	return archive, identity, nil
}

type PackPreview struct {
	Info     modpack.Info
	Version  packcatalog.Version
	Deferred bool
}

func (s *Service) PreviewPack(ctx context.Context, version packcatalog.Version) (PackPreview, error) {
	preview := PackPreview{Version: version}
	if version.URL == "" {
		return preview, errors.New(locales.DownloadThisFileInBrowserThenImportIt())
	}
	reader, err := s.client.OpenRange(ctx, version.URL, version.Size)
	if err == nil {
		if version.Size > 0 && version.Size != reader.Size() {
			return preview, errors.New(locales.TheModpackChangedSelectItAgain())
		}
		preview.Version.Size = reader.Size()
		preview.Info, err = modpack.Preview(ctx, reader, reader.Size(), packFilename(version))
	}
	if errors.Is(err, download.ErrRangeUnsupported) || errors.Is(err, download.ErrPreviewLimit) || errors.Is(err, modpack.ErrPreviewDeferred) {
		preview.Deferred = true
		format := "ZIP"
		switch version.Source {
		case packcatalog.Modrinth:
			format = "Modrinth"
		case packcatalog.CurseForge:
			format = "CurseForge"
		}
		name := version.Name
		if name == "" {
			filename := packFilename(version)
			name = strings.TrimSuffix(filename, filepath.Ext(filename))
		}
		preview.Info = modpack.Info{Format: format, Name: name, Version: version.Number}
		for _, game := range version.GameVersions {
			if len(game) > 0 && game[0] >= '0' && game[0] <= '9' {
				preview.Info.GameVersion = game
				break
			}
		}
		for _, loader := range version.Loaders {
			preview.Info.Loaders = append(preview.Info.Loaders, modpack.Loader{ID: loader})
		}
		return preview, nil
	}
	return preview, err
}

func packFilename(v packcatalog.Version) string {
	if fsutil.ValidateID(v.Filename) == nil {
		return v.Filename
	}
	return "pack.zip"
}

func (s *Service) packCachePath(v packcatalog.Version) string {
	return cache.FileIn(s.cacheDir, "downloads", v.URL+"|"+v.SHA1+"|"+v.SHA512)
}

func (s *Service) FetchPack(ctx context.Context, v packcatalog.Version, progress func(download.Progress)) (string, error) {
	if v.URL == "" {
		return "", errors.New(locales.DownloadThisFileInBrowserThenImportIt())
	}
	path := s.packCachePath(v)
	if err := s.client.FetchAll(ctx, []download.Task{{URL: v.URL, Path: path, SHA1: v.SHA1, SHA512: v.SHA512, Size: v.Size}}, 1, progress); err != nil {
		return "", err
	}
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	archive, err := fsutil.OpenZip(ctx, path)
	if err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf(locales.UrlDidNotReturnAZipMrpackFile(), err)
	}
	if err := archive.Close(); err != nil {
		return "", err
	}
	return path, nil
}

type PackCover struct {
	Image     image.Image
	Animation []byte
}

func (s *Service) PackCover(ctx context.Context, raw string) (PackCover, error) {
	path := cache.FileIn(s.cacheDir, "images", raw)
	var lastErr error
	for range 2 {
		if err := s.client.Fetch(ctx, download.Task{URL: raw, Path: path, MaxSize: 8 << 20}); err != nil {
			return PackCover{}, err
		}
		release, err := s.client.FilePermit(ctx)
		if err != nil {
			return PackCover{}, err
		}
		f, err := fsutil.OpenRegular(path)
		if err == nil {
			data, readErr := io.ReadAll(io.LimitReader(fsutil.ContextReader(ctx, f), (8<<20)+1))
			_ = f.Close()
			release()
			if readErr == nil {
				if icon, decodeErr := minecraft.DecodeInstanceIcon(data); decodeErr == nil {
					animation, _ := minecraft.AnimatedIcon(data)
					return PackCover{Image: icon, Animation: animation}, nil
				} else {
					lastErr = decodeErr
				}
			} else {
				lastErr = readErr
			}
		} else {
			release()
			return PackCover{}, err
		}
		_ = os.Remove(path)
	}
	return PackCover{}, lastErr
}
