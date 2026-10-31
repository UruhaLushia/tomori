package launcher

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

type PackUpdateRequest struct {
	Path                 string
	Optional             bool
	LocalFiles           []string
	Source               minecraft.PackSource
	Archive              modpack.ArchiveIdentity
	AllowDifferentSource bool
	Preview              *PackPreview
}

type PackUpdate struct {
	Plan                            *modpack.UpdatePlan
	mu                              sync.Mutex
	instance                        minecraft.Instance
	root, content, stage, candidate *os.Root
	metadata                        *os.Root
	stamp, rootStamp, contentStamp  os.FileInfo
	name, previousSHA1              string
	components                      [32]byte
	next                            *modpack.Origin
	profile                         *minecraft.VersionDetail
	finished                        bool
}

func packUpdateChanged() error {
	return errors.New(locales.InstanceFilesChangedCheckTheModpackUpdateAgain())
}

func packRootCurrent(root *os.Root, expected os.FileInfo) error {
	if root == nil || expected == nil {
		return packUpdateChanged()
	}
	current, err := os.Stat(root.Name())
	if err != nil {
		return err
	}
	held, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !current.IsDir() || !os.SameFile(current, held) || !os.SameFile(current, expected) {
		return packUpdateChanged()
	}
	return nil
}

func (p *PackUpdate) current() error {
	if p.root == nil || p.finished {
		return packUpdateChanged()
	}
	if err := packRootCurrent(p.root, p.rootStamp); err != nil {
		return err
	}
	if err := packRootCurrent(p.content, p.contentStamp); err != nil {
		return err
	}
	return packRootCurrent(p.stage, p.stamp)
}

func (p *PackUpdate) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.root == nil {
		return nil
	}
	var problem error
	for _, root := range []*os.Root{p.candidate, p.metadata, p.stage, p.content} {
		if root != nil {
			problem = errors.Join(problem, root.Close())
		}
	}
	if p.stamp != nil {
		current, err := p.root.Lstat(p.name)
		if err == nil && os.SameFile(current, p.stamp) && current.IsDir() {
			err = p.root.RemoveAll(p.name)
		} else if err == nil {
			err = packUpdateChanged()
		}
		problem = errors.Join(problem, err)
	}
	problem = errors.Join(problem, p.root.Close())
	p.root, p.content, p.stage, p.candidate, p.metadata = nil, nil, nil, nil, nil
	p.next, p.profile = nil, nil
	return problem
}

func (s *Service) PreparePackUpdate(ctx context.Context, instance minecraft.Instance, request PackUpdateRequest, progress func(minecraft.Progress)) (_ *PackUpdate, err error) {
	s = s.forInstance(instance)
	if err := modpack.ValidateSource(request.Source); err != nil {
		return nil, err
	}
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return nil, err
	}
	defer release()
	components, err := minecraft.InspectComponents(ctx, instance)
	if err != nil {
		return nil, err
	}
	if components.Reason != "" {
		return nil, errors.New(components.Reason)
	}
	p := &PackUpdate{instance: instance, components: components.Fingerprint, name: ".tomori-pack-next-" + rand.Text()}
	defer func() {
		if err != nil && p.root != nil {
			err = errors.Join(err, p.Close())
		}
	}()
	p.root, err = minecraft.OpenInstanceRoot(ctx, instance)
	if err != nil {
		return nil, err
	}
	p.rootStamp, err = p.root.Stat(".")
	if err != nil {
		return nil, err
	}
	previous, digest, err := modpack.ReadOriginSnapshot(ctx, p.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New(locales.MissingModpackOriginRecord())
	}
	if err != nil {
		return nil, err
	}
	p.previousSHA1 = digest
	if preview := request.Preview; preview != nil {
		v := preview.Version
		if request.Source.Provider != v.Source || request.Source.ProjectID != v.ProjectID || request.Source.VersionID != v.ID {
			return nil, errors.New(locales.TheModpackChangedSelectItAgain())
		}
		request.Path, err = s.FetchPack(ctx, v, func(d download.Progress) {
			if progress != nil {
				progress(minecraft.Progress{Stage: "packfiles", Download: &d})
			}
		})
		if err != nil {
			return nil, err
		}
	}
	a, identity, err := s.openPack(ctx, request.Path)
	if err != nil {
		return nil, err
	}
	defer a.Close()
	if preview := request.Preview; preview != nil && !preview.Deferred && !reflect.DeepEqual(preview.Info, a.Info) {
		return nil, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	if request.Archive.SHA512 != "" && (request.Archive.Size != identity.Size || !strings.EqualFold(request.Archive.SHA512, identity.SHA512)) {
		return nil, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	if len(a.Info.Warnings) > 0 {
		return nil, errors.New(strings.Join(a.Info.Warnings, "；"))
	}
	if old := previous.Header.Source; !request.AllowDifferentSource && old.Provider != "" && request.Source.Provider != "" && (old.Provider != request.Source.Provider || old.ProjectID != request.Source.ProjectID) {
		return nil, errors.New(locales.TheSelectedPackHasADifferentSource())
	}
	if instance.Format == minecraft.InstancePrism && a.Profile != nil && len(a.Info.Loaders) == 0 {
		return nil, errors.New(locales.ThisModpackHasCustomVersionSettingsChooseARegularGameDirectoryInsteadOfPrism())
	}
	p.next, err = modpack.NewOrigin(a.Info, request.Source, identity, request.Optional)
	if err != nil {
		return nil, err
	}
	p.profile = a.Profile
	files, err := modpack.Resolve(ctx, s.client, a.Info, modpack.CurseForgeKey(s.cfg.Download.CurseForgeKey), request.Optional)
	if err != nil {
		return nil, err
	}
	resolved := a.Info
	resolved.Files = files
	needs, err := s.packContentSpaceRequirements(ctx, instance.InstanceDir, resolved, true)
	if err != nil {
		return nil, err
	}
	space, err := storage.Assess(ctx, needs)
	if err != nil {
		return nil, err
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "space", Space: &space})
	}
	if err := space.Check(); err != nil {
		return nil, err
	}
	sources, err := s.preparePackFiles(ctx, files, request.LocalFiles, progress)
	if err != nil {
		return nil, err
	}
	content := instance.ContentDirectory(s.cfg.Game.Isolate)
	canonical, err := filepath.EvalSymlinks(content)
	if err != nil || canonical != filepath.Clean(content) {
		return nil, packUpdateChanged()
	}
	p.content, err = os.OpenRoot(content)
	if err != nil {
		return nil, err
	}
	p.contentStamp, err = p.content.Stat(".")
	if err != nil {
		return nil, err
	}
	if err := packRootCurrent(p.root, p.rootStamp); err != nil {
		return nil, err
	}
	if err := p.root.Mkdir(p.name, 0o700); err != nil {
		return nil, err
	}
	p.stamp, err = p.root.Lstat(p.name)
	if err != nil {
		return nil, err
	}
	p.stage, err = p.root.OpenRoot(p.name)
	if err != nil {
		return nil, err
	}
	if err := p.current(); err != nil {
		return nil, err
	}
	for _, name := range []string{"content", "config"} {
		if err := p.stage.Mkdir(name, 0o700); err != nil {
			return nil, err
		}
	}
	p.candidate, err = p.stage.OpenRoot("content")
	if err != nil {
		return nil, err
	}
	p.metadata, err = p.stage.OpenRoot("config")
	if err != nil {
		return nil, err
	}
	if err := modpack.StageFiles(ctx, p.candidate, files, sources, s.client.FilePermit, p.next.Put); err != nil {
		return nil, err
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "pack-extract"})
	}
	permit, err := s.client.FilePermit(ctx)
	if err != nil {
		return nil, err
	}
	err = a.ExtractRoot(ctx, p.candidate, nil, p.next.Put)
	permit()
	if err != nil {
		return nil, err
	}
	if err := p.next.Write(ctx, p.metadata); err != nil {
		return nil, err
	}
	p.Plan, err = modpack.PlanUpdate(ctx, p.content, previous, p.next, s.client.FilePermit, func(done, total int) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-compare", FilesDone: done, Files: total})
		}
	})
	if err != nil {
		return nil, err
	}
	return p, p.current()
}

func (p *PackUpdate) AssessSpace(ctx context.Context, choices map[string]modpack.UpdateChoice) (storage.Report, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.current(); err != nil {
		return storage.Report{}, err
	}
	return p.Plan.AssessSpace(ctx, p.content, choices)
}

func (s *Service) ApplyPackUpdate(ctx context.Context, p *PackUpdate, choices map[string]modpack.UpdateChoice, progress func(minecraft.Progress), onLine func(string)) (backup string, err error) {
	if p == nil {
		return "", packUpdateChanged()
	}
	choices = maps.Clone(choices)
	p.mu.Lock()
	defer p.mu.Unlock()
	s = s.forInstance(p.instance)
	release, err := s.lockRoot(ctx, p.instance.GameDir, progress)
	if err != nil {
		return "", err
	}
	defer release()
	if err := p.current(); err != nil {
		return "", err
	}
	if filepath.Clean(p.content.Name()) != filepath.Clean(p.instance.ContentDirectory(s.cfg.Game.Isolate)) {
		return "", packUpdateChanged()
	}
	beforeWrite, finishRecovery := s.packRecoveryTracker(ctx, p.instance, p.content.Name())
	defer func() { err = errors.Join(err, finishRecovery()) }()
	var kind modpack.Loader
	if len(p.next.Header.Pack.Loaders) > 0 {
		kind = p.next.Header.Pack.Loaders[0]
	}
	err = s.prepareComponents(ctx, p.instance, p.components, p.next.Header.Pack.GameVersion, kind, p.profile, progress, onLine, func(detail *minecraft.VersionDetail, raw []byte) (err error) {
		if err := p.current(); err != nil {
			return err
		}
		metadata, err := s.packUpdateMetadata(ctx, p, detail, raw)
		if err != nil {
			return err
		}
		plan, err := modpack.PlanMetadata(ctx, p.root, p.metadata, metadata, s.client.FilePermit)
		if err != nil {
			return err
		}
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-update"})
		}
		tx, err := p.Plan.Apply(ctx, p.content, p.candidate, choices, s.client.FilePermit, func(done, total int) {
			if progress != nil {
				progress(minecraft.Progress{Stage: "pack-update", FilesDone: done, Files: total})
			}
		}, beforeWrite)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, tx.Close()) }()
		if progress != nil {
			progress(minecraft.Progress{Stage: "pack-update-config"})
		}
		if err := tx.ApplyMetadata(ctx, plan, p.root, p.metadata); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		backup = tx.BackupPath()
		p.finished = true
		return nil
	})
	return backup, err
}

func (s *Service) packUpdateMetadata(ctx context.Context, p *PackUpdate, detail *minecraft.VersionDetail, prism []byte) ([]modpack.MetadataFile, error) {
	files := []modpack.MetadataFile{{Path: modpack.OriginFile, ExpectedSHA1: p.previousSHA1, CheckExpected: true}}
	if detail != nil || prism != nil {
		component, err := minecraft.PrepareComponentFile(ctx, p.instance, p.components, detail, prism)
		if err != nil {
			return nil, err
		}
		if err := fsutil.WriteRootAtomic(ctx, p.metadata, component.Path, component.Raw); err != nil {
			return nil, err
		}
		files = append(files, modpack.MetadataFile{Path: component.Path, ExpectedSHA1: component.ExpectedSHA1, CheckExpected: true})
	}
	name := minecraft.PrismMetadataFile
	if p.instance.Format == minecraft.InstanceManaged {
		name = minecraft.InstanceMetadata
	}
	previous, err := fsutil.ReadRootFile(p.root, name, 16<<20)
	missing := errors.Is(err, os.ErrNotExist) && p.instance.Format != minecraft.InstanceManaged
	if err != nil && !missing {
		return nil, err
	}
	fields := make(map[string]json.RawMessage)
	if !missing {
		if err := json.Unmarshal(previous, &fields); err != nil {
			return nil, err
		}
		if fields == nil {
			return nil, errors.New(locales.InvalidOrOversizedInstanceConfiguration())
		}
	}
	pack := p.next.Header.Pack
	for key, value := range map[string]any{"packFormat": pack.Format, "packVersion": pack.Version, "packSource": p.next.Header.Source} {
		fields[key], err = json.Marshal(value)
		if err != nil {
			return nil, err
		}
	}
	if p.instance.Format == minecraft.InstanceManaged {
		var loaders []string
		for _, loader := range pack.Loaders {
			loaders = append(loaders, loader.ID)
		}
		for key, value := range map[string]any{"gameVersion": pack.GameVersion, "loader": (modpack.Info{Loaders: pack.Loaders}).LoaderLabel(), "loaders": loaders} {
			fields[key], _ = json.Marshal(value)
		}
	}
	raw, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := fsutil.WriteRootAtomic(ctx, p.metadata, name, raw); err != nil {
		return nil, err
	}
	expected := ""
	if !missing {
		hash := sha1.Sum(previous)
		expected = hex.EncodeToString(hash[:])
	}
	return append(files, modpack.MetadataFile{Path: name, ExpectedSHA1: expected, CheckExpected: true}), nil
}
