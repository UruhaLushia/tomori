package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/serverstatus"
	"github.com/UruhaLushia/tomori/locales"
)

type ServerTarget struct {
	Address  string
	Protocol int
}

func (s *Service) Servers(ctx context.Context, instance minecraft.Instance) (instancecontent.ServerList, error) {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return instancecontent.ServerList{}, err
	}
	defer release()
	return instancecontent.ReadServers(ctx, instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate))
}

func (s *Service) ChangeServer(ctx context.Context, instance minecraft.Instance, revision [32]byte, index int, next instancecontent.Server, remove bool) error {
	if !instance.CanLaunch() {
		return errors.New(locales.InvalidTargetInstance())
	}
	release, err := s.lockRoot(ctx, instance.GameDir, nil)
	if err != nil {
		return err
	}
	defer release()
	releaseFile, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer releaseFile()
	return instancecontent.ChangeServer(ctx, instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate), revision, index, next, remove)
}

func (s *Service) ServerProtocol(ctx context.Context, instance minecraft.Instance) int {
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return -1
	}
	defer release()
	if instance.Format != minecraft.InstancePrism {
		if detail, err := minecraft.ReadLocalVersion(instance.GameDir, instance.ID); err == nil {
			if protocol := clientProtocol(ctx, filepath.Join(instance.GameDir, "versions", detail.ClientID(), detail.ClientID()+".jar")); protocol >= 0 {
				return protocol
			}
		}
	}
	if fsutil.ValidateID(instance.GameVersion) != nil {
		return -1
	}
	roots := []string{instance.GameDir}
	if instance.Format == minecraft.InstancePrism {
		roots = prismRoots(instance)
	}
	for _, root := range roots {
		for _, path := range []string{filepath.Join(root, "versions", instance.GameVersion, instance.GameVersion+".jar"), filepath.Join(root, "libraries", "com", "mojang", "minecraft", instance.GameVersion, "minecraft-"+instance.GameVersion+"-client.jar")} {
			if protocol := clientProtocol(ctx, path); protocol >= 0 {
				return protocol
			}
		}
	}
	return -1
}

func clientProtocol(ctx context.Context, path string) int {
	z, err := fsutil.OpenZip(ctx, path)
	if err != nil {
		return -1
	}
	defer z.Close()
	f, err := fsutil.FindZipFile(z.Reader, "version.json")
	if err != nil {
		return -1
	}
	raw, err := fsutil.ReadZipFile(ctx, f, 64<<10)
	if err != nil {
		return -1
	}
	var metadata struct {
		Protocol *int `json:"protocol_version"`
	}
	if json.Unmarshal(raw, &metadata) != nil || metadata.Protocol == nil || *metadata.Protocol < 0 {
		return -1
	}
	return *metadata.Protocol
}

func QuickPlaySupported(version string) bool {
	if year, week, ok := snapshotVersion(version); ok {
		return year > 13 || year == 13 && week >= 16
	}
	major, rest, ok := strings.Cut(version, ".")
	if !ok {
		return false
	}
	minor, _, _ := strings.Cut(rest, ".")
	minor, _, _ = strings.Cut(minor, "-")
	a, errA := strconv.Atoi(major)
	b, errB := strconv.Atoi(minor)
	return errA == nil && errB == nil && (a > 1 || a == 1 && b >= 6)
}

func snapshotVersion(version string) (int, int, bool) {
	if len(version) < 6 || version[2] != 'w' {
		return 0, 0, false
	}
	year, errA := strconv.Atoi(version[:2])
	week, errB := strconv.Atoi(version[3:5])
	return year, week, errA == nil && errB == nil && week > 0 && week <= 53
}

func (c LaunchConfig) modernQuickPlay() bool {
	version := c.Version.MinecraftVersion
	if version == "" {
		version = c.Version.ClientID()
	}
	if year, week, ok := snapshotVersion(version); ok {
		return year > 23 || year == 23 && week >= 14
	}
	major, rest, _ := strings.Cut(version, ".")
	minor, _, _ := strings.Cut(rest, ".")
	minor, _, _ = strings.Cut(minor, "-")
	a, errA := strconv.Atoi(major)
	b, errB := strconv.Atoi(minor)
	return errA == nil && errB == nil && (a > 1 || a == 1 && b >= 20)
}

func (c LaunchConfig) serverArguments(game []string) ([]string, error) {
	if c.Server.Address == "" {
		return game, nil
	}
	address, err := serverstatus.ParseAddress(c.Server.Address)
	if err != nil {
		return nil, err
	}
	modern := c.modernQuickPlay()
	out := game[:0]
	for i := 0; i < len(game); i++ {
		switch game[i] {
		case "--quickPlayMultiplayer":
			modern = true
			i++
		case "--server", "--port", "--quickPlaySingleplayer", "--quickPlayRealms":
			i++
		default:
			out = append(out, game[i])
		}
	}
	if modern {
		return append(out, "--quickPlayMultiplayer", address.String()), nil
	}
	return append(out, "--server", address.Host, "--port", strconv.Itoa(int(address.Port))), nil
}
