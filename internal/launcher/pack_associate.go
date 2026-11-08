package launcher

import (
	"context"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/internal/packcatalog"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type PackAssociation struct {
	Version             packcatalog.Version
	Path                string
	Archive             modpack.ArchiveIdentity
	Preview             *PackPreview
	Optional, Confirmed bool
}

func associationSource(v packcatalog.Version) (minecraft.PackSource, error) {
	source := minecraft.PackSource{Provider: v.Source, ProjectID: v.ProjectID, VersionID: v.ID, Number: v.Number}
	if source.Provider == "" {
		return source, errors.New(locales.ChooseAModrinthOrCurseForgeModpackVersion())
	}
	return source, modpack.ValidateSource(source)
}

func (s *Service) ResolvePackReference(ctx context.Context, link string) (packcatalog.Version, []packcatalog.Version, error) {
	v, versions, err := (packcatalog.Client{Fetcher: s.http}).ResolveReference(ctx, link)
	if err != nil {
		return v, nil, err
	}
	if _, err := associationSource(v); err != nil {
		return v, nil, err
	}
	if versions == nil {
		versions, err = s.PackVersions(ctx, v.Source, v.ProjectID, "")
	}
	if err != nil {
		if ctx.Err() != nil {
			return v, nil, ctx.Err()
		}
		versions = []packcatalog.Version{v}
	}
	found := false
	for _, version := range versions {
		found = found || version.ID == v.ID
	}
	if !found {
		versions = append([]packcatalog.Version{v}, versions...)
	}
	return v, versions, nil
}

func associationCompatible(components minecraft.InstanceComponents, info modpack.Info) error {
	if components.Reason != "" {
		return errors.New(components.Reason)
	}
	if components.Game != info.GameVersion || len(info.Loaders) > 1 {
		return errors.New(locales.ModpackGameVersionOrLoaderDoesNotMatchThisInstance())
	}
	loader := modpack.Loader{}
	if len(info.Loaders) > 0 {
		loader = info.Loaders[0]
	}
	current := strings.TrimPrefix(components.LoaderVersion, components.Game+"-")
	selected := strings.TrimPrefix(loader.Version, info.GameVersion+"-")
	if components.Loader != loader.ID || current != selected {
		return errors.New(locales.ModpackGameVersionOrLoaderDoesNotMatchThisInstance())
	}
	return nil
}

func missingPackOrigin(root *os.Root) error {
	_, err := root.Lstat(modpack.OriginFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New(locales.ThisInstanceAlreadyHasAModpackRecordUseUpdateModpack())
}

func (s *Service) AssociatePack(ctx context.Context, instance minecraft.Instance, request PackAssociation, progress func(minecraft.Progress)) (modpack.OriginHeader, error) {
	var empty modpack.OriginHeader
	if !request.Confirmed {
		return empty, errors.New(locales.ConfirmTheCurrentlyInstalledModpackVersion())
	}
	source, err := associationSource(request.Version)
	if err != nil {
		return empty, err
	}
	s = s.forInstance(instance)
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return empty, err
	}
	defer release()
	permit, err := s.client.FilePermit(ctx)
	if err != nil {
		return empty, err
	}
	root, err := minecraft.OpenInstanceRoot(ctx, instance)
	var stamp os.FileInfo
	var components minecraft.InstanceComponents
	if err == nil {
		stamp, err = root.Stat(".")
	}
	if err == nil {
		err = missingPackOrigin(root)
	}
	if err == nil {
		components, err = minecraft.InspectExportComponents(ctx, instance)
	}
	permit()
	if root != nil {
		defer root.Close()
	}
	if err != nil {
		return empty, err
	}
	if components.Reason != "" {
		return empty, errors.New(components.Reason)
	}
	path := request.Path
	if path == "" {
		if progress != nil {
			progress(minecraft.Progress{Stage: "packfiles"})
		}
		path, err = s.FetchPack(ctx, request.Version, func(p download.Progress) {
			if progress != nil {
				progress(minecraft.Progress{Stage: "packfiles", Download: &p})
			}
		})
		if err != nil {
			return empty, err
		}
	}
	a, identity, err := s.openPack(ctx, path, true, request.Archive)
	if err != nil {
		return empty, err
	}
	defer a.Close()
	if request.Archive.SHA512 != "" && (request.Archive.Size != identity.Size || request.Archive.SHA512 != identity.SHA512) {
		return empty, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	if preview := request.Preview; preview != nil {
		v := preview.Version
		if v.Source != source.Provider || v.ProjectID != source.ProjectID || v.ID != source.VersionID || !preview.Deferred && !reflect.DeepEqual(preview.Info, a.Info) {
			return empty, errors.New(locales.TheModpackChangedSelectItAgain())
		}
	}
	if err := associationCompatible(components, a.Info); err != nil {
		return empty, err
	}
	permit, err = s.client.FilePermit(ctx)
	if err != nil {
		return empty, err
	}
	err = a.VerifySource(ctx, request.Version.Size, request.Version.SHA1, request.Version.SHA512)
	permit()
	if err != nil {
		return empty, err
	}
	if request.Path != "" && request.Version.SHA1 == "" && request.Version.SHA512 == "" {
		return empty, errors.New(locales.ThisVersionHasNoFileChecksums())
	}
	files, err := modpack.Resolve(ctx, s.http, a.Info, modpack.CurseForgeKey(s.cfg.Download.CurseForgeKey), request.Optional)
	if err != nil {
		return empty, err
	}
	origin, err := modpack.NewOrigin(a.Info, source, identity, request.Optional)
	if err != nil {
		return empty, err
	}
	permit, err = s.client.FilePermit(ctx)
	if err != nil {
		return empty, err
	}
	defer permit()
	if progress != nil {
		progress(minecraft.Progress{Stage: "pack-compare"})
	}
	if err := a.RecordBaseline(ctx, origin, files); err != nil {
		return empty, err
	}
	current, err := minecraft.InspectExportComponents(ctx, instance)
	if err != nil {
		return empty, err
	}
	if current.Fingerprint != components.Fingerprint {
		return empty, packUpdateChanged()
	}
	if err := packRootCurrent(root, stamp); err != nil {
		return empty, err
	}
	if err := missingPackOrigin(root); err != nil {
		return empty, err
	}
	report, err := storage.Assess(ctx, []storage.Requirement{{Directory: root.Name(), Bytes: origin.EstimatedSize() * 2}})
	if err != nil {
		return empty, err
	}
	if err := report.Check(); err != nil {
		return empty, err
	}
	stage := ".tomori-pack-associate-" + rand.Text()
	if err := root.Mkdir(stage, 0700); err != nil {
		return empty, err
	}
	defer func() { _ = root.RemoveAll(stage) }()
	candidate, err := root.OpenRoot(stage)
	if err != nil {
		return empty, err
	}
	err = origin.Write(ctx, candidate)
	_ = candidate.Close()
	if err != nil {
		return empty, err
	}
	if err := packRootCurrent(root, stamp); err != nil {
		return empty, err
	}
	if err := fsutil.PublishRootExclusive(ctx, root, filepath.Join(stage, modpack.OriginFile), modpack.OriginFile, nil); err != nil {
		return empty, err
	}
	return origin.Header, nil
}
