package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/account"
	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/java"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/modpack"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

// Service coordinates a single job using an immutable settings snapshot.
// It does not own UI state. The caller controls its lifetime through context.
type Service struct {
	cfg                          config.Config
	cacheDir, runtimeDir, logDir string
	client                       *download.Client
	store                        *config.Store
	windowedDisplaySizes         [][2]int
}

func New(cfg config.Config, store *config.Store, budget ...*download.Budget) *Service {
	client := download.NewClient(cfg.Download.Proxy)
	if len(budget) > 0 {
		client.Budget = budget[0]
	} else {
		client.Budget = download.NewBudget(cfg.Download.Workers, max(cfg.Download.Workers, cfg.Download.Connections))
		client.Budget.SetSpeedLimit(int64(cfg.Download.SpeedLimitKiB) << 10)
	}
	client.UserAgent = "UruhaLushia/Tomori/0.1.0"
	client.CacheDir = cfg.CacheDirectory()
	key := modpack.CurseForgeKey(cfg.Download.CurseForgeKey)
	client.PrepareRequest = func(req *http.Request) {
		// Never forward an API key to an unrelated redirect destination.
		req.Header.Del("x-api-key")
		if req.URL.Scheme != "https" {
			return
		}
		switch req.URL.Hostname() {
		case "api.curseforge.com", "edge.forgecdn.net", "media.forgecdn.net", "mediafilez.forgecdn.net", "files.forgecdn.net":
			if key != "" {
				req.Header.Set("x-api-key", key)
			}
		}
	}
	client.Rewrite = minecraft.ParseSource(cfg.Download.Source).Rewrite
	client.Workers = max(1, min(cfg.Download.Workers, 32))
	client.Connections = max(1, min(cfg.Download.Connections, 32))
	client.Retries = max(0, min(cfg.Download.Retries, 10))
	client.Segmented = cfg.Download.Segmented
	client.SegmentThreshold = int64(max(1, min(cfg.Download.SegmentThresholdMB, 1024))) << 20
	client.HTTP.Timeout = time.Duration(max(10, cfg.Download.TimeoutSeconds)) * time.Second
	return &Service{cfg: cfg.Clone(), cacheDir: cfg.CacheDirectory(), runtimeDir: store.RuntimeDir(), logDir: store.LogDir(), client: client, store: store}
}

func (s *Service) Close() { s.client.Close() }

func (s *Service) discoverJava(ctx context.Context) []java.Runtime {
	paths := slices.Clone(s.cfg.Java.ManualPaths)
	paths = append(paths, s.runtimeDir)
	for _, directory := range s.cfg.Directories.Entries {
		paths = append(paths, filepath.Join(directory.Path, "runtime"))
	}
	return java.Discover(ctx, "", paths...)
}

func (s *Service) DownloadJava(ctx context.Context, request java.DownloadRequest, progress func(minecraft.Progress)) (java.Runtime, error) {
	return java.Download(ctx, s.client, request, s.runtimeDir, progress)
}

func (s *Service) lockRoot(ctx context.Context, root string, progress func(minecraft.Progress)) (func(), error) {
	if progress != nil {
		progress(minecraft.Progress{Stage: "waiting"})
	}
	return s.client.Lock(ctx, "game-write", root)
}

func (s *Service) forInstance(instance minecraft.Instance) *Service {
	job := *s
	if settings, ok := job.cfg.Instances[instance.Key()]; ok && settings.Enabled {
		job.cfg.Game, job.cfg.Java = settings.Game, settings.Java
	}
	return &job
}

// Catalog returns a cached catalog alongside a warning when the network is down.
func (s *Service) Catalog(ctx context.Context) (*minecraft.Manifest, error) {
	path := filepath.Join(s.cacheDir, download.ManifestCache)
	m, err := minecraft.FetchManifest(ctx, s.client)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		raw, readErr := os.ReadFile(path)
		var cached minecraft.Manifest
		if readErr == nil && json.Unmarshal(raw, &cached) == nil && len(cached.Versions) > 0 {
			return &cached, fmt.Errorf(locales.NetworkRequestFailedUsingCachedData(), err)
		}
		return nil, err
	}
	raw, marshalErr := json.Marshal(m)
	if marshalErr != nil {
		return m, marshalErr
	}
	if err := fsutil.WriteAtomic(path, raw); err != nil {
		return m, fmt.Errorf(locales.FailedToCacheVersionManifest(), err)
	}
	return m, nil
}

func (s *Service) Install(ctx context.Context, root string, v minecraft.VersionSummary, progress func(minecraft.Progress)) (minecraft.Instance, error) {
	parent, prism, err := minecraft.PrismInstallRoot(ctx, root)
	if err != nil {
		return minecraft.Instance{}, fmt.Errorf(locales.ReadingInstallLocation(), err)
	}
	if prism {
		return s.installPrism(ctx, parent, v, progress)
	}
	release, err := s.lockRoot(ctx, root, progress)
	if err != nil {
		return minecraft.Instance{}, err
	}
	defer release()
	detail, err := minecraft.FetchVersion(ctx, s.client, v)
	if err != nil {
		return minecraft.Instance{}, err
	}
	if err := s.installer(root, progress).Install(ctx, detail); err != nil {
		return minecraft.Instance{}, err
	}
	path := filepath.Join(root, "versions", v.ID)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return minecraft.Instance{
		VersionSummary: v, Name: v.ID, GameVersion: v.ID,
		GameDir: root, InstanceDir: path, Format: minecraft.InstanceMinecraft,
	}, nil
}

func (s *Service) Repair(ctx context.Context, instance minecraft.Instance, progress func(minecraft.Progress)) error {
	s = s.forInstance(instance)
	if !instance.CanLaunch() {
		return errors.New(locales.UnsupportedInstanceFormatAlt())
	}
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return err
	}
	defer release()
	root, detail, err := s.instanceVersion(ctx, instance)
	if err != nil {
		return err
	}
	installer := s.instanceInstaller(instance, root, progress)
	return installer.Repair(ctx, detail)
}

func (s *Service) installer(root string, progress func(minecraft.Progress)) *minecraft.Installer {
	return &minecraft.Installer{Fetcher: s.client, GameDir: root, Isolate: s.cfg.Game.Isolate, Env: minecraft.CurrentEnvironment(), Progress: progress, ReuseRoots: s.installReuseRoots()}
}

func (s *Service) installReuseRoots(preferred ...string) []string {
	var roots []string
	add := func(directory string) {
		if directory == "" {
			return
		}
		path := filepath.Clean(directory)
		if !slices.Contains(roots, path) {
			roots = append(roots, path)
		}
		if strings.EqualFold(filepath.Base(path), "instances") {
			parent := filepath.Dir(path)
			if !slices.Contains(roots, parent) {
				roots = append(roots, parent)
			}
		}
	}
	for _, directory := range preferred {
		add(directory)
	}
	for _, entry := range s.cfg.Directories.Entries {
		add(entry.Path)
	}
	return roots
}

func installClientSources(game string, roots []string) []string {
	sources := make([]string, 0, len(roots)*2)
	for _, root := range roots {
		sources = append(sources,
			filepath.Join(root, "versions", game, game+".jar"),
			filepath.Join(root, "libraries", "com", "mojang", "minecraft", game, "minecraft-"+game+"-client.jar"))
	}
	return sources
}

func (s *Service) Launch(ctx context.Context, instance minecraft.Instance, identity account.Account, list []java.Runtime, progress func(minecraft.Progress), onLine func(string), targets ...ServerTarget) (*Process, java.Runtime, error) {
	if len(targets) > 0 {
		if _, err := serverstatus.ParseAddress(targets[0].Address); err != nil {
			return nil, java.Runtime{}, err
		}
		if !QuickPlaySupported(instance.GameVersion) {
			return nil, java.Runtime{}, errors.New(locales.ThisVersionDoesNotSupportQuickJoin())
		}
	}
	cfg, j, err := s.PrepareLaunch(ctx, instance, identity, list, progress)
	if err != nil {
		return nil, j, err
	}
	if len(targets) > 0 {
		cfg.Server = targets[0]
		release, err := s.client.FilePermit(ctx)
		if err != nil {
			return nil, j, err
		}
		protocol := clientProtocol(ctx, cfg.clientJar())
		release()
		if protocol < 0 {
			protocol = s.ServerProtocol(ctx, instance)
		}
		if !(serverstatus.Status{Protocol: cfg.Server.Protocol}).Compatible(protocol) {
			return nil, j, errors.New(locales.ServerAndInstanceVersionsAreIncompatible())
		}
	}
	if err := os.MkdirAll(s.logDir, 0o755); err != nil {
		return nil, j, err
	}
	logPath := filepath.Join(s.logDir, cfg.Version.ID+"-"+time.Now().Format("20060102-150405.000000000")+".log")
	if progress != nil {
		progress(minecraft.Progress{Stage: "launch"})
	}
	logs, err := s.store.LogStore()
	if err != nil {
		return nil, j, err
	}
	cfg.Progress = progress
	p, err := Run(ctx, *cfg, logPath, onLine, logs)
	return p, j, err
}

// PrepareLaunch resolves and repairs the instance without starting a game process.
func (s *Service) PrepareLaunch(ctx context.Context, instance minecraft.Instance, identity account.Account, list []java.Runtime, progress func(minecraft.Progress)) (*LaunchConfig, java.Runtime, error) {
	s = s.forInstance(instance)
	if !instance.CanLaunch() {
		return nil, java.Runtime{}, errors.New(locales.UnsupportedInstanceFormatAlt())
	}
	if err := identity.Validate(); err != nil {
		return nil, java.Runtime{}, err
	}
	if err := s.cfg.Game.Execution.Validate(); err != nil {
		return nil, java.Runtime{}, err
	}
	extra, err := SplitArguments(s.cfg.Game.ExtraJVM)
	if err != nil {
		return nil, java.Runtime{}, err
	}
	release, err := s.lockRoot(ctx, instance.GameDir, progress)
	if err != nil {
		return nil, java.Runtime{}, err
	}
	defer release()
	if progress != nil {
		progress(minecraft.Progress{Stage: "metadata"})
	}
	root, detail, err := s.instanceVersion(ctx, instance)
	if err != nil {
		return nil, java.Runtime{}, err
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "java-check"})
	}
	customJava := s.cfg.Java.RuntimePath(detail.RequiredJava())
	if len(list) == 0 && customJava == "" {
		list = s.discoverJava(ctx)
	}
	j, err := java.Ensure(ctx, s.client, detail, list, customJava, s.runtimeDir, s.cfg.Java.AutoDownload, s.cfg.Java.IgnoreCompatibility, progress)
	if err != nil {
		return nil, java.Runtime{}, err
	}
	env := minecraft.CurrentEnvironment()
	if j.Arch != "" {
		env.Arch = j.Arch
	}
	installer := s.instanceInstaller(instance, root, progress)
	installer.Env = env
	// Check hashes using the selected Java architecture before generating arguments.
	if err := installer.Repair(ctx, detail); err != nil {
		return nil, j, fmt.Errorf(locales.PreLaunchRepairFailed(), err)
	}
	gameDir := instance.ContentDirectory(s.cfg.Game.Isolate)
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		return nil, j, err
	}
	memoryMiB, err := launchMemory(ctx, s.cfg.Game, j.Arch)
	if err != nil {
		return nil, j, err
	}
	cfg := LaunchConfig{
		Version: detail, RootDir: root, GameDir: gameDir, AssetsDir: installer.AssetsDir(),
		LibrariesDir: installer.LibrariesDir(), ClientJar: installer.ClientJar(detail.ClientID()),
		InstanceDir: instance.InstanceDir, InstanceName: instance.Name, Execution: s.cfg.Game.Execution,
		NativesDir: installer.NativesDir(detail.ID), JavaPath: j.Path,
		Account: identity, MemoryMiB: memoryMiB, FullScreen: s.cfg.Game.FullScreen,
		Width: s.cfg.Game.Width, Height: s.cfg.Game.Height, ExtraJVM: extra,
		ProcessPriority: s.cfg.Game.ProcessPriority,
		LauncherName:    "tomori", LauncherVer: "0.1.0", Env: env,
	}
	if identity.Kind == account.Yggdrasil {
		if progress != nil {
			progress(minecraft.Progress{Stage: "auth"})
		}
		if err := s.prepareYggdrasil(ctx, &cfg); err != nil {
			return nil, j, fmt.Errorf(locales.CouldNotPrepareAuthenticationAgent(), err)
		}
	}
	cfg.avoidImplicitFullscreen(s.windowedDisplaySizes)
	if detail.Logging != nil && detail.Logging.Client != nil {
		cfg.LogConfig = filepath.Join(installer.AssetsDir(), "log_configs", detail.Logging.Client.File.ID())
	}
	if detail.AssetIndex.URL != "" {
		var index struct {
			MapToResources bool `json:"map_to_resources"`
		}
		raw, err := os.ReadFile(filepath.Join(installer.AssetsDir(), "indexes", detail.AssetIndex.ID+".json"))
		if err != nil {
			return nil, j, err
		}
		if err := json.Unmarshal(raw, &index); err != nil {
			return nil, j, err
		}
		if index.MapToResources {
			cfg.GameAssetsDir = filepath.Join(gameDir, "resources")
		}
	}
	return &cfg, j, nil
}

func (s *Service) instanceVersion(ctx context.Context, instance minecraft.Instance) (string, *minecraft.VersionDetail, error) {
	if err := ctx.Err(); err != nil {
		return "", nil, err
	}
	if instance.Format == minecraft.InstancePrism {
		return s.preparePrism(ctx, instance)
	}
	detail, err := minecraft.ReadLocalVersion(instance.GameDir, instance.ID)
	return instance.GameDir, detail, err
}

func (s *Service) instanceInstaller(instance minecraft.Instance, root string, progress func(minecraft.Progress)) *minecraft.Installer {
	installer := s.installer(root, progress)
	installer.Isolate = instance.Format == minecraft.InstanceMinecraft && s.cfg.Game.Isolate
	if instance.Format == minecraft.InstancePrism {
		installer.ContentDir = instance.GameDir
		installer.ReuseRoots = s.installReuseRoots(append([]string{root}, prismRoots(instance)...)...)
		installer.ClientSources = installClientSources(instance.GameVersion, installer.ReuseRoots)
		shared := prismSharedRoot(instance)
		installer.LibraryRoot, installer.AssetRoot = filepath.Join(shared, "libraries"), filepath.Join(shared, "assets")
		installer.ClientPath = filepath.Join(installer.LibraryRoot, "com", "mojang", "minecraft", instance.GameVersion, "minecraft-"+instance.GameVersion+"-client.jar")
	}
	return installer
}
