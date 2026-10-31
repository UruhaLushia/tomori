package minecraft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

// DiscoverDirectories merges registered roots and removes overlapping installations.
// A failed root does not hide instances found in the other directories.
func DiscoverDirectories(ctx context.Context, roots []string) ([]Instance, []error) {
	if err := ctx.Err(); err != nil {
		return nil, []error{err}
	}
	type result struct {
		instances []Instance
		warnings  []error
		err       error
	}
	results := make([]result, len(roots))
	workers := min(len(roots), 4)
	if workers > 0 {
		jobs := make(chan int)
		var wg sync.WaitGroup
		wg.Add(workers)
		for range workers {
			go func() {
				defer wg.Done()
				for i := range jobs {
					if ctx.Err() != nil {
						return
					}
					instances, warnings, err := DiscoverInstances(ctx, roots[i])
					results[i] = result{instances: instances, warnings: warnings, err: err}
				}
			}()
		}
	queue:
		for i := range roots {
			select {
			case jobs <- i:
			case <-ctx.Done():
				break queue
			}
		}
		close(jobs)
		wg.Wait()
	}

	var list []Instance
	var problems []error
	seen := make(map[string]bool)
	for i, root := range roots {
		result := results[i]
		for _, warning := range result.warnings {
			problems = append(problems, fmt.Errorf("%s: %w", root, warning))
		}
		if result.err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", root, result.err))
		}
		for _, instance := range result.instances {
			if seen[instance.Key()] {
				continue
			}
			seen[instance.Key()] = true
			instance.SourceDir = root
			list = append(list, instance)
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, append(problems, err)
	}
	slices.SortStableFunc(list, func(a, b Instance) int {
		if order := b.ReleaseTime.Compare(a.ReleaseTime); order != 0 {
			return order
		}
		if order := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); order != 0 {
			return order
		}
		return strings.Compare(a.Key(), b.Key())
	})
	return list, problems
}

// DiscoverInstances works without a remote manifest, including imported versions.
func DiscoverInstances(ctx context.Context, gameDir string) ([]Instance, []error, error) {
	var list []Instance
	var problems, failures []error
	for _, discover := range []func(context.Context, string) ([]Instance, []error, error){
		discoverMinecraftInstances, discoverPrismInstances, discoverManagedInstances,
	} {
		if err := ctx.Err(); err != nil {
			return nil, problems, err
		}
		instances, warnings, err := discover(ctx, gameDir)
		list, problems = append(list, instances...), append(problems, warnings...)
		if err != nil {
			failures = append(failures, err)
		}
	}
	for i := range list {
		if err := ctx.Err(); err != nil {
			return nil, problems, err
		}
		if path, err := filepath.EvalSymlinks(list[i].InstanceDir); err == nil {
			list[i].InstanceDir = path
		}
		if list[i].Format == InstanceMinecraft {
			if root, err := os.OpenRoot(list[i].InstanceDir); err == nil {
				raw, readErr := fsutil.ReadRootFile(root, instanceDisplayFile, 1<<20)
				_ = root.Close()
				var display struct{ Name string }
				if readErr == nil && json.Unmarshal(raw, &display) == nil && strings.TrimSpace(display.Name) != "" {
					list[i].Name = display.Name
				}
			}
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].ReleaseTime.Equal(list[j].ReleaseTime) {
			return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name)
		}
		return list[i].ReleaseTime.After(list[j].ReleaseTime)
	})
	if err := ctx.Err(); err != nil {
		return nil, problems, err
	}
	return list, problems, errors.Join(failures...)
}

func discoverMinecraftInstances(ctx context.Context, gameDir string) ([]Instance, []error, error) {
	root, err := os.OpenRoot(gameDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = root.Close() }()
	entries, err := fs.ReadDir(root.FS(), "versions")
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var list []Instance
	var problems []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		id := entry.Name()
		v, err := readLocalVersion(root, id, make(map[string]bool))
		if errors.Is(err, os.ErrNotExist) {
			continue
		} // unfinished downloads aren't instances
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", id, err))
			continue
		}
		gameVersion := ProfileGameVersion(v, "")
		loaders, loaderLabel := localLoaders(v, gameVersion)
		list = append(list, Instance{
			VersionSummary: VersionSummary{ID: id, Type: v.Type, Time: v.Time, ReleaseTime: v.ReleaseTime},
			Name:           id, GameVersion: gameVersion, GameDir: gameDir,
			Loaders: loaders, Loader: loaderLabel,
			InstanceDir: filepath.Join(gameDir, "versions", id), Format: InstanceMinecraft,
		})
	}
	return list, problems, nil
}

func ProfileGameVersion(v *VersionDetail, fallback string) string {
	if v.MinecraftVersion != "" {
		return v.MinecraftVersion
	}
	if game := localGameArgument(v, "--fml.mcVersion"); game != "" {
		return game
	}
	if v.InheritsFrom != "" {
		return v.InheritsFrom
	}
	if fallback != "" {
		return fallback
	}
	return v.ID
}

func localGameArgument(v *VersionDetail, name string) string {
	pending := false
	read := func(value string) (string, bool) {
		if pending {
			return value, true
		}
		if value == name {
			pending = true
		} else if value, ok := strings.CutPrefix(value, name+"="); ok {
			return value, true
		}
		return "", false
	}
	for value := range strings.FieldsSeq(v.MinecraftArguments) {
		if result, ok := read(value); ok {
			return result
		}
	}
	for _, arg := range v.Arguments.Game {
		if len(arg.Rules) != 0 {
			continue
		}
		for _, value := range arg.Value {
			if result, ok := read(value); ok {
				return result
			}
		}
	}
	return ""
}

func localLoaders(v *VersionDetail, game string) ([]string, string) {
	var ids, labels []string
	for _, item := range ProfileLoaders(v, game) {
		ids = append(ids, ComponentLoader(item.ID))
		labels = append(labels, item.Name+" "+item.Version)
	}
	return ids, strings.Join(labels, " · ")
}

func ProfileLoaders(v *VersionDetail, game string) []InstalledComponent {
	var result []InstalledComponent
	for _, library := range v.Libraries {
		group, coordinate, ok := strings.Cut(library.Name, ":")
		if !ok {
			continue
		}
		artifact, version, ok := strings.Cut(coordinate, ":")
		if !ok {
			continue
		}
		version, _, _ = strings.Cut(version, ":")
		var id, label string
		version = strings.TrimPrefix(version, game+"-")
		switch group + ":" + artifact {
		case "net.fabricmc:fabric-loader":
			id, label = "fabric", "Fabric"
		case "org.quiltmc:quilt-loader":
			id, label = "quilt", "Quilt"
		case "net.minecraftforge:forge", "net.minecraftforge:fmlloader":
			id, label = "forge", "Forge"
		case "net.neoforged:neoforge", "net.neoforged:forge":
			id, label = "neoforge", "NeoForge"
		case "net.neoforged.fancymodloader:loader":
			id, label = "neoforge", "NeoForge"
			version = localGameArgument(v, "--fml.neoForgeVersion")
		}
		if id != "" && version != "" && !slices.ContainsFunc(result, func(item InstalledComponent) bool { return item.ID == LoaderUID(id) }) {
			result = append(result, InstalledComponent{ID: LoaderUID(id), Name: label, Version: version})
		}
	}
	return result
}
