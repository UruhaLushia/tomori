package launcher

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modrinth"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func resourceFilter(instance minecraft.Instance, kind resource.Kind, category string) modrinth.Filter {
	loaders := instance.Loaders
	if len(loaders) == 0 && instance.Loader != "" {
		fields := strings.Fields(instance.Loader)
		if len(fields) > 0 {
			loaders = []string{strings.ToLower(fields[0])}
		}
	}
	filter := modrinth.Filter{Kind: kind, GameVersion: instance.GameVersion, Loaders: loaders}
	if category != "" {
		filter.Categories = []string{category}
	}
	return filter
}

func (s *Service) SearchResources(ctx context.Context, instance minecraft.Instance, kind resource.Kind, source resourcecatalog.Source, compatibility resourcecatalog.CompatibilityFilter, category, query string, offset int) (resourcecatalog.SearchResult, error) {
	filter := compatibility.Apply(resourceFilter(instance, kind, category))
	return (resourcecatalog.Client{Fetcher: s.client}).Search(ctx, source, query, filter, offset)
}

func (s *Service) TranslateDescriptions(ctx context.Context, projects []resourcecatalog.Project) []resourcecatalog.Project {
	return (resourcecatalog.Client{Fetcher: s.client}).TranslateDescriptions(ctx, projects)
}

func (s *Service) ResourceVersions(ctx context.Context, kind resource.Kind, project resourcecatalog.Project) ([]resourcecatalog.Version, error) {
	return (resourcecatalog.Client{Fetcher: s.client}).Versions(ctx, project, resourcecatalog.Filter{Kind: kind})
}

func (s *Service) ResourceVersionDetails(ctx context.Context, version resourcecatalog.Version) (resourcecatalog.VersionDetails, error) {
	return (resourcecatalog.Client{Fetcher: s.client}).Details(ctx, version)
}

func (s *Service) InstallResource(ctx context.Context, instance minecraft.Instance, kind resource.Kind, version resourcecatalog.Version, progress func(minecraft.Progress)) (int, error) {
	return s.installResourceVersions(ctx, instance, kind, []resourcecatalog.Version{version}, nil, progress, "", resource.InstallDefault)
}

func (s *Service) InstallResourceWithPolicy(ctx context.Context, instance minecraft.Instance, kind resource.Kind, version resourcecatalog.Version, policy resource.InstallPolicy, progress func(minecraft.Progress)) (int, error) {
	if kind == resource.World || kind == resource.DataPack {
		return s.installDownloadOnly(ctx, instance, kind, version, policy, progress)
	}
	return s.installResourceVersions(ctx, instance, kind, []resourcecatalog.Version{version}, nil, progress, "", policy)
}

// installDownloadOnly handles resources that are downloaded to the instance's
// saves directory rather than managed as enabled/disabled library files.
func (s *Service) installDownloadOnly(ctx context.Context, instance minecraft.Instance, kind resource.Kind, version resourcecatalog.Version, policy resource.InstallPolicy, progress func(minecraft.Progress)) (int, error) {
	if !instance.CanLaunch() || !kind.Valid() || !version.Source.Valid() || version.ID == "" || version.ProjectID == "" {
		return 0, errors.New(locales.InvalidResourceSourceOrType())
	}
	s = s.forInstance(instance)
	filter := resourceFilter(instance, kind, "")
	var plan []resourcecatalog.InstallFile
	var err error
	if version.Source == resourcecatalog.CurseForge {
		plan, err = (resourcecatalog.Client{Fetcher: s.client}).ResolveCurse(ctx, version, filter, nil)
	} else {
		resolved, resolveErr := (modrinth.Client{Fetcher: s.client}).Resolve(ctx, version.ID, filter, nil)
		err = resolveErr
		if err == nil {
			plan = resourcecatalog.ModrinthPlan(resolved)
		}
	}
	if err != nil {
		return 0, err
	}
	if len(plan) == 0 {
		return 0, errors.New(locales.InvalidResourceSourceOrType())
	}
	for _, item := range plan {
		if item.Kind != kind {
			return 0, errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
		}
	}
	tasks, err := resourceDownloadTasks(s.cacheDir, plan)
	if err != nil {
		return 0, err
	}
	if err := s.client.FetchAll(ctx, tasks, 0, func(p download.Progress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "download", Done: p.Done, Total: p.Total, Download: &p})
		}
	}); err != nil {
		return 0, err
	}
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return 0, err
	}
	defer release()
	destination := filepath.Join(instance.ContentDirectory(s.cfg.Game.Isolate), "saves")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return 0, err
	}
	for i, item := range plan {
		target := filepath.Join(destination, item.Name)
		if _, statErr := os.Stat(target); statErr == nil {
			if policy == resource.InstallDefault {
				return 0, fmt.Errorf(locales.AlreadyExistsRemoveTheExistingVersionFirst(), item.Name)
			}
			if err := os.Remove(target); err != nil {
				return 0, err
			}
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return 0, statErr
		}
		if err := fsutil.Publish(ctx, tasks[i].Path, target); err != nil {
			return 0, err
		}
	}
	return len(plan), nil
}

func (s *Service) InstallWorldPack(ctx context.Context, instance minecraft.Instance, folder string, version resourcecatalog.Version, progress func(minecraft.Progress)) (int, error) {
	if folder == "" || !version.Source.Valid() {
		return 0, errors.New(locales.InvalidResourceSourceOrType())
	}
	return s.installResourceVersions(ctx, instance, resource.DataPack, []resourcecatalog.Version{version}, nil, progress, folder, resource.InstallDefault)
}

type ResourceUpdate = resourcecatalog.ResourceUpdate

func (s *Service) UpdateResources(ctx context.Context, instance minecraft.Instance, kind resource.Kind, updates []ResourceUpdate, progress func(minecraft.Progress)) (int, error) {
	versions, err := resourceUpdateVersions(updates)
	if err != nil {
		return 0, err
	}
	return s.installResourceVersions(ctx, instance, kind, versions, updates, progress, "", resource.InstallReplace)
}

func (s *Service) UpdateWorldPacks(ctx context.Context, instance minecraft.Instance, folder string, updates []ResourceUpdate, progress func(minecraft.Progress)) (int, error) {
	versions, err := resourceUpdateVersions(updates)
	if err != nil {
		return 0, err
	}
	return s.installResourceVersions(ctx, instance, resource.DataPack, versions, updates, progress, folder, resource.InstallReplace)
}

func resourceUpdateVersions(updates []ResourceUpdate) ([]resourcecatalog.Version, error) {
	if len(updates) == 0 || len(updates) > 1000 {
		return nil, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	versions := make([]resourcecatalog.Version, len(updates))
	seen := make(map[string]bool, len(updates))
	for i, update := range updates {
		v := update.Version
		if v.Source != resourcecatalog.Source(update.Origin.Source) || v.ProjectID != update.Origin.ProjectID || v.ID == update.Origin.VersionID || update.SHA1 == "" || seen[v.ProjectID] {
			return nil, errors.New(locales.ResourceChangedRefreshTheList())
		}
		seen[v.ProjectID] = true
		versions[i] = v
	}
	return versions, nil
}

type previousResource struct {
	entry resource.Entry
	kind  resource.Kind
	hash  string
}

func (s *Service) installResourceVersions(ctx context.Context, instance minecraft.Instance, kind resource.Kind, versions []resourcecatalog.Version, updates []ResourceUpdate, progress func(minecraft.Progress), folder string, policy resource.InstallPolicy) (int, error) {
	if kind == resource.DataPack && folder == "" {
		return 0, errors.New(locales.ManageDataPacksWithinAWorld())
	}
	s = s.forInstance(instance)
	if len(versions) == 0 || len(versions) > 1000 || !kind.Valid() || !versions[0].Source.Valid() {
		return 0, errors.New(locales.InvalidResourceType())
	}
	source := versions[0].Source
	replace := policy != resource.InstallDefault || updates != nil
	reinstallRoot := updates == nil && policy != resource.InstallDefault
	for _, v := range versions {
		if v.Source != source || v.ID == "" || v.ProjectID == "" {
			return 0, errors.New(locales.InvalidResourceSourceOrType())
		}
	}
	if kind == resource.Mod && len(instance.Loaders) == 0 {
		return 0, errors.New(locales.NoModLoaderInstalled())
	}
	root := instance.ContentDirectory(s.cfg.Game.Isolate)
	var world *worldDirectory
	var release func()
	var err error
	if kind == resource.DataPack {
		world, release, err = s.openWorldResources(ctx, instance, folder, progress)
		if err == nil {
			root = world.path
		}
	} else {
		release, err = s.lockRoot(ctx, instance.GameDir, progress)
	}
	if err != nil {
		return 0, err
	}
	defer release()
	var packs instancecontent.WorldPacks
	if world != nil {
		unlock, err := s.client.FilePermit(ctx)
		if err != nil {
			return 0, err
		}
		packs, err = instancecontent.ReadWorldPacks(ctx, world.root)
		unlock()
		if err != nil {
			return 0, err
		}
		if packs.Version != "" {
			instance.GameVersion = packs.Version
		}
	}
	origins, err := resource.LoadOrigins(s.store.Dir(), root)
	if err != nil {
		return 0, err
	}
	var originalOrigins *resource.Origins
	if world != nil {
		originalOrigins = &resource.Origins{Version: origins.Version, Files: slices.Clone(origins.Files)}
	}
	knownSizes := make(map[resource.Kind]map[int64]bool)
	for _, file := range origins.Files {
		if knownSizes[file.Kind] == nil {
			knownSizes[file.Kind] = make(map[int64]bool)
		}
		knownSizes[file.Kind][file.Size] = true
	}
	present := make(map[string]bool)
	client := modrinth.Client{Fetcher: s.client}
	local := make(map[resource.Kind][]resource.Entry)
	hashEntries := make(map[string]resource.Entry)
	hashKinds := make(map[string]resource.Kind)
	hashCounts := make(map[string]int)
	previous := make(map[string]previousResource)
	projectCounts := make(map[string]int)
	identifiedOrigins := make(map[string]resource.Origin)
	var hashes []string
	var fingerprints []resourcecatalog.Fingerprint
	kinds := []resource.Kind{resource.Mod, resource.ResourcePack, resource.ShaderPack}
	if world != nil {
		kinds = []resource.Kind{resource.DataPack}
	}
	for _, current := range kinds {
		entries, err := resource.Scan(ctx, root, current)
		if err != nil {
			return 0, err
		}
		if world != nil {
			for i := range entries {
				entries[i].Enabled = packs.IsEnabled(entries[i].File)
			}
		}
		local[current] = entries
		for _, entry := range entries {
			if entry.Directory {
				continue
			}
			identify := current == kind || current == resource.Mod
			if !identify && !knownSizes[current][entry.Size] {
				continue
			}
			fingerprint := source == resourcecatalog.CurseForge && identify
			hash, value, err := s.resourceHash(ctx, root, current, entry, fingerprint)
			if err != nil {
				return 0, err
			}
			if fingerprint {
				fingerprints = append(fingerprints, value)
			}
			present[string(current)+":"+hash] = true
			if !identify {
				continue
			}
			hashCounts[hash]++
			if source == resourcecatalog.Modrinth {
				hashes = append(hashes, hash)
			}
			if previous, ok := hashEntries[hash]; !ok || !previous.Enabled {
				hashEntries[hash] = entry
				hashKinds[hash] = current
			}
		}
	}
	var plan []resourcecatalog.InstallFile
	filter := resourceFilter(instance, kind, "")
	if source == resourcecatalog.CurseForge {
		catalog := resourcecatalog.Client{Fetcher: s.client}
		installed, err := catalog.IdentifyCurse(ctx, fingerprints)
		if err != nil {
			return 0, err
		}
		for project, file := range installed {
			identifiedOrigins[file.SHA1] = resource.Origin{Source: string(resourcecatalog.CurseForge), ProjectID: project, VersionID: file.VersionID, Number: file.Number}
			previous[project] = previousResource{entry: hashEntries[file.SHA1], kind: hashKinds[file.SHA1], hash: file.SHA1}
			projectCounts[project] = file.Copies
		}
		if updates != nil {
			if err := validateResourceUpdates(updates, kind, previous, identifiedOrigins, projectCounts); err != nil {
				return 0, err
			}
		}
		switch {
		case reinstallRoot:
			plan, err = catalog.ResolveCurseReinstall(ctx, versions[0], filter, installed)
		case replace:
			plan, err = catalog.ResolveCurseUpdates(ctx, versions, filter, installed)
		default:
			plan, err = catalog.ResolveCurse(ctx, versions[0], filter, installed)
		}
		if err != nil {
			return 0, err
		}
	} else {
		identified, err := client.Identify(ctx, hashes)
		if err != nil {
			return 0, err
		}
		installed := make(map[string]modrinth.Installed)
		for hash, version := range identified {
			entry, known := hashEntries[hash]
			if !known {
				continue
			}
			matches := false
			for _, file := range version.Files {
				if strings.EqualFold(file.Hashes["sha1"], hash) && file.Size == entry.Size {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
			identifiedOrigins[hash] = resource.Origin{Source: string(resourcecatalog.Modrinth), ProjectID: version.ProjectID, VersionID: version.ID, Number: version.Number}
			projectCounts[version.ProjectID] += hashCounts[hash]
			if previous, ok := installed[version.ProjectID]; ok {
				if previous.Enabled && entry.Enabled && previous.Version.ID != version.ID {
					return 0, fmt.Errorf(locales.MultipleVersionsOfAreEnabledRemoveDuplicatesFirst(), entry.Name)
				}
				if previous.Enabled {
					continue
				}
			}
			installed[version.ProjectID] = modrinth.Installed{Version: version, Enabled: entry.Enabled}
			previous[version.ProjectID] = previousResource{entry: entry, kind: hashKinds[hash], hash: hash}
		}
		var resolved []modrinth.InstallFile
		if updates != nil {
			if err := validateResourceUpdates(updates, kind, previous, identifiedOrigins, projectCounts); err != nil {
				return 0, err
			}
		}
		switch {
		case reinstallRoot:
			resolved, err = client.ResolveReinstall(ctx, versions[0].ID, filter, installed)
		case replace:
			ids := make([]string, len(versions))
			for i, v := range versions {
				ids[i] = v.ID
			}
			resolved, err = client.ResolveUpdates(ctx, ids, filter, installed)
		default:
			resolved, err = client.Resolve(ctx, versions[0].ID, filter, installed)
		}
		if err != nil {
			return 0, err
		}
		plan = resourcecatalog.ModrinthPlan(resolved)
	}
	for _, version := range versions {
		for _, item := range plan {
			if item.Version.ID == version.ID && item.Version.ProjectID != version.ProjectID {
				return 0, errors.New(locales.ReturnedResourceVersionDoesNotMatchTheRequest())
			}
		}
	}
	if world != nil {
		var size int64
		for _, item := range plan {
			if item.Kind != resource.DataPack {
				return 0, errors.New(locales.ResourceTypeDoesNotMatchTheCurrentPage())
			}
			if item.Size < 0 || item.Size > 4<<30-size {
				return 0, errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
			}
			size += item.Size
		}
	}
	if replace {
		for _, item := range plan {
			if old, ok := previous[item.Version.ProjectID]; ok {
				if projectCounts[item.Version.ProjectID] != 1 || old.kind != item.Kind {
					return 0, fmt.Errorf(locales.HasDuplicateVersionsRemoveDuplicatesFirst(), old.entry.Name)
				}
				delete(present, string(old.kind)+":"+old.hash)
				delete(identifiedOrigins, old.hash)
			}
		}
	}
	origins.Retain(func(file resource.OriginFile) bool { return present[string(file.Kind)+":"+file.SHA1] })
	for hash, origin := range identifiedOrigins {
		if entry, ok := hashEntries[hash]; ok {
			if err := origins.Remember(resource.OriginFile{Kind: hashKinds[hash], SHA1: hash, Size: entry.Size, Origin: origin}); err != nil {
				return 0, err
			}
		}
	}
	for _, item := range plan {
		v := item.Version
		if err := origins.Remember(resource.OriginFile{Kind: item.Kind, SHA1: item.SHA1, Size: item.Size, Origin: resource.Origin{Source: string(v.Source), ProjectID: v.ProjectID, VersionID: v.ID, Number: v.Number}}); err != nil {
			return 0, err
		}
	}
	tasks, err := resourceDownloadTasks(s.cacheDir, plan)
	if err != nil {
		return 0, err
	}
	missing := &resourcecatalog.MissingFilesError{}
	needs, err := s.client.SpaceRequirements(ctx, tasks)
	if err != nil {
		return 0, err
	}
	for _, item := range plan {
		bytes := storage.Add(uint64(item.Size), uint64(item.Size))
		if old, ok := previous[item.Version.ProjectID]; replace && ok {
			bytes = storage.Add(bytes, uint64(max(0, old.entry.Size)))
		}
		needs = append(needs, storage.Requirement{Directory: filepath.Join(root, string(item.Kind)), Bytes: bytes})
	}
	space, err := storage.Assess(ctx, needs)
	if progress != nil {
		progress(minecraft.Progress{Stage: "space", Space: &space})
	}
	if err != nil {
		return 0, err
	}
	if err := space.Check(); err != nil {
		return 0, err
	}
	networkTasks := make([]download.Task, 0, len(tasks))
	for i, task := range tasks {
		item := plan[i]
		if !item.Manual {
			networkTasks = append(networkTasks, task)
			continue
		}
		if err := s.client.ReuseLocal(ctx, task, nil); err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			missing.Files = append(missing.Files, resourcecatalog.MissingFile{Name: item.Name, Website: item.Version.Website, SHA1: item.SHA1, Size: item.Size, Kind: item.Kind})
		}
	}
	if len(missing.Files) > 0 {
		return 0, missing
	}
	if err := s.client.FetchAll(ctx, networkTasks, 0, func(p download.Progress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "download", Done: p.Done, Total: p.Total, Download: &p})
		}
	}); err != nil {
		return 0, err
	}
	releaseFile, err := s.client.FilePermit(ctx)
	if err != nil {
		return 0, err
	}
	defer releaseFile()
	if err := validateResourceFiles(ctx, plan, tasks, local, previous, replace); err != nil {
		return 0, err
	}
	var changes []resource.Change
	for i, item := range plan {
		change := resource.Change{Kind: item.Kind, Path: tasks[i].Path, SHA1: item.SHA1, Disabled: item.Disabled}
		if world != nil {
			change.Disabled = false
		}
		if old, ok := previous[item.Version.ProjectID]; replace && ok {
			change.Previous, change.PreviousSHA1 = &old.entry, old.hash
			change.PreservePrevious = policy == resource.InstallKeepDisabled
		}
		changes = append(changes, change)
	}
	var transaction *resource.Transaction
	if len(changes) > 0 {
		transaction, err = resource.ApplyChanges(ctx, root, changes)
		if err != nil {
			return 0, err
		}
		defer transaction.Close()
	}
	originsSaved := false
	rollback := func(cause error) error {
		if transaction != nil {
			cause = errors.Join(cause, transaction.Rollback())
		}
		if originsSaved && originalOrigins != nil {
			cause = errors.Join(cause, originalOrigins.Save(s.store.Dir(), root))
		}
		return cause
	}
	if err := ctx.Err(); err != nil {
		return 0, rollback(err)
	}
	if origins.Changed() {
		if err := origins.Save(s.store.Dir(), root); err != nil {
			return 0, rollback(fmt.Errorf(locales.FailedToSaveResourceOrigins(), err))
		}
		originsSaved = true
	}
	if world != nil {
		if err := world.Check(); err != nil {
			return 0, rollback(err)
		}
		changes := make(map[string]int, len(plan))
		renames := make(map[string]string)
		for _, item := range plan {
			if old, ok := previous[item.Version.ProjectID]; replace && ok && old.entry.File != item.Name {
				renames[old.entry.File] = item.Name
			}
			changes[item.Name] = 1
			if item.Disabled {
				changes[item.Name] = -1
			}
		}
		if err := instancecontent.ReplaceWorldPacks(ctx, world.root, changes, renames); err != nil {
			return 0, rollback(err)
		}
	}
	if transaction != nil {
		if err := transaction.Commit(); err != nil {
			return len(changes), fmt.Errorf(locales.ResourcesInstalledCouldNotCleanStagingFiles(), err)
		}
	}
	return len(changes), nil
}

func validateResourceUpdates(updates []ResourceUpdate, kind resource.Kind, previous map[string]previousResource, origins map[string]resource.Origin, counts map[string]int) error {
	for _, update := range updates {
		old, exists := previous[update.Origin.ProjectID]
		if !exists || old.hash != update.SHA1 || old.kind != kind || old.entry.Relative != update.Entry.Relative || old.entry.Enabled != update.Entry.Enabled || origins[old.hash] != update.Origin {
			return errors.New(locales.ResourceChangedRefreshTheList())
		}
		if counts[update.Origin.ProjectID] != 1 {
			return fmt.Errorf(locales.HasDuplicateVersionsRemoveDuplicatesFirst(), old.entry.Name)
		}
	}
	return nil
}

func (s *Service) resourceHash(ctx context.Context, root string, kind resource.Kind, entry resource.Entry, fingerprint bool) (string, resourcecatalog.Fingerprint, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return "", resourcecatalog.Fingerprint{}, err
	}
	defer release()
	if fingerprint {
		value, err := resourcecatalog.EntryFingerprint(ctx, root, kind, entry)
		return value.SHA1, value, err
	}
	hash, err := resource.HashEntry(ctx, root, kind, entry)
	return hash, resourcecatalog.Fingerprint{}, err
}

func resourceDownloadTasks(cacheDir string, plan []resourcecatalog.InstallFile) ([]download.Task, error) {
	var tasks []download.Task
	for _, item := range plan {
		if err := fsutil.ValidateID(item.Name); err != nil {
			return nil, err
		}
		if !item.Kind.Accepts(item.Name, false) || item.Size <= 0 {
			return nil, errors.New(locales.UnsupportedFileTypeOrSize())
		}
		hash := strings.ToLower(item.SHA1)
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 20 {
			return nil, fmt.Errorf(locales.HasNoValidChecksum(), item.Name)
		}
		uri, err := url.Parse(item.URL)
		allowed := err == nil && uri.Scheme == "https" && uri.User == nil && uri.Port() == ""
		if allowed {
			if item.Version.Source == resourcecatalog.CurseForge {
				allowed = slices.Contains([]string{"edge.forgecdn.net", "media.forgecdn.net", "mediafilez.forgecdn.net", "files.forgecdn.net"}, uri.Hostname())
			} else {
				allowed = uri.Hostname() == "cdn.modrinth.com"
			}
		}
		if item.Manual && item.Version.Source == resourcecatalog.CurseForge && item.URL == "" {
			allowed = true
		}
		if !allowed {
			return nil, errors.New(locales.InvalidResourceDownloadUrl())
		}
		path := filepath.Join(cacheDir, download.ResourceCache, hash, item.Name)
		tasks = append(tasks, download.Task{URL: item.URL, Path: path, SHA1: hash, Size: item.Size})
	}
	return tasks, nil
}

func validateResourceFiles(ctx context.Context, plan []resourcecatalog.InstallFile, tasks []download.Task, local map[resource.Kind][]resource.Entry, previous map[string]previousResource, replace bool) error {
	identities := make(map[string]bool)
	names := make(map[string]bool, len(plan))
	for i, item := range plan {
		candidate, err := resource.Inspect(ctx, tasks[i].Path, item.Kind)
		if err != nil {
			return fmt.Errorf("%s: %w", item.Name, err)
		}
		for _, entry := range local[item.Kind] {
			if old, ok := previous[item.Version.ProjectID]; replace && ok && old.kind == item.Kind && entry.Relative == old.entry.Relative {
				continue
			}
			if strings.EqualFold(entry.File, item.Name) || (candidate.ID != "" && candidate.ID == entry.ID) {
				return fmt.Errorf(locales.AlreadyExistsRemoveTheExistingVersionFirst(), entry.Name)
			}
		}
		if candidate.ID != "" {
			identity := string(item.Kind) + ":" + candidate.ID
			if identities[identity] {
				return fmt.Errorf(locales.ModAlreadyExists(), candidate.ID)
			}
			identities[identity] = true
		}
		name := string(item.Kind) + ":" + strings.ToLower(item.Name)
		if names[name] {
			return fmt.Errorf(locales.DuplicateDownloadFilename(), item.Name)
		}
		names[name] = true
	}
	return nil
}
