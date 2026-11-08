package launcher

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/config"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type prismMetadata struct {
	UID, Version         string
	FormatVersion        int
	MainJar              minecraft.Library
	CompatibleJavaMajors []int
	JVMArgs              []string            `json:"+jvmArgs"`
	Tweakers             []string            `json:"+tweakers"`
	Traits               []string            `json:"+traits"`
	Libraries            []minecraft.Library `json:"+libraries"`
	Requires             []struct{ UID, Equals, Suggests string }
	MCVersion            string `json:"mcVersion"`
	detail               minecraft.VersionDetail
}

func (s *Service) preparePrism(ctx context.Context, instance minecraft.Instance) (string, *minecraft.VersionDetail, error) {
	pack, err := minecraft.ReadPrismPack(instance.InstanceDir)
	if err != nil {
		return "", nil, err
	}
	return s.preparePrismPack(ctx, instance, pack)
}

// Use Prism's component profiles and ForgeWrapper so installed files follow
// the same validation and patching path as a launch from Prism itself.
func (s *Service) preparePrismPack(ctx context.Context, instance minecraft.Instance, pack minecraft.PrismPack) (string, *minecraft.VersionDetail, error) {
	components := slices.DeleteFunc(slices.Clone(pack.Components), func(c minecraft.PrismComponent) bool { return c.Disabled })
	versions := make(map[string]string)
	loaders, lwjgl := 0, 0
	for _, component := range components {
		versions[component.UID] = component.Version
		switch component.UID {
		case "net.minecraft", "net.minecraft.java", "net.java", "net.fabricmc.intermediary":
		case "org.lwjgl", "org.lwjgl3":
			lwjgl++
		case "net.fabricmc.fabric-loader", "org.quiltmc.quilt-loader", "net.minecraftforge", "net.neoforged":
			loaders++
		}
	}
	if loaders > 1 {
		return "", nil, errors.New(locales.ThisPrismInstanceContainsMultipleModLoaders())
	}
	if lwjgl > 1 {
		return "", nil, errors.New(locales.InstanceContainsConflictingLwjglComponents())
	}
	game := versions["net.minecraft"]
	if game == "" {
		return "", nil, errors.New(locales.InstanceHasNoMinecraftVersion())
	}
	instance.GameVersion = game
	shared := prismSharedRoot(instance)
	var metadata []prismMetadata
	for index := 0; index < len(components); index++ {
		component := components[index]
		if component.UID == "net.java" || component.UID == "net.minecraft.java" {
			continue // Java is selected once by PrepareLaunch.
		}
		meta, err := s.prismMetadata(ctx, shared, instance.InstanceDir, component)
		if err != nil {
			return "", nil, err
		}
		for _, required := range meta.Requires {
			version := versions[required.UID]
			if version == "" {
				switch required.UID {
				case "org.lwjgl", "org.lwjgl3":
					if lwjgl == 0 {
						version = cmp.Or(required.Equals, required.Suggests)
						lwjgl++
					}
				case "net.fabricmc.intermediary":
					version = game
				}
				if version != "" {
					components = append(components, minecraft.PrismComponent{UID: required.UID, Version: version})
					versions[required.UID] = version
				}
			}
			if version == "" || required.Equals != "" && required.Equals != version {
				return "", nil, fmt.Errorf(locales.PrismComponentDependencyMismatch(), required.UID)
			}
		}
		metadata = append(metadata, *meta)
	}
	detail := &minecraft.VersionDetail{ID: game, Jar: game, MinecraftVersion: game}
	for _, value := range []string{"-Djava.library.path=${natives_directory}", "-cp", "${classpath}"} {
		detail.Arguments.JVM = append(detail.Arguments.JVM, minecraft.Argument{Value: minecraft.ArgumentValue{value}})
	}
	for _, meta := range metadata {
		meta.detail.CompatibleJavaMajors = slices.Clone(meta.CompatibleJavaMajors)
		if meta.UID == "net.minecraft" {
			client, err := meta.MainJar.Artifact()
			if (err != nil || client == nil || client.URL == "") && meta.detail.Downloads.Client.URL != "" {
				client, err = &meta.detail.Downloads.Client, nil
			}
			if err != nil || client == nil || client.URL == "" || len(client.SHA1) != 40 || len(meta.CompatibleJavaMajors) == 0 {
				return "", nil, errors.New(locales.MinecraftComponentMetadataIsIncomplete())
			}
			meta.detail.Downloads.Client = *client
			meta.detail.JavaVersion = &minecraft.JavaTarget{MajorVersion: slices.Min(meta.CompatibleJavaMajors)}
		}
		for _, value := range meta.JVMArgs {
			meta.detail.Arguments.JVM = append(meta.detail.Arguments.JVM, minecraft.Argument{Value: minecraft.ArgumentValue{value}})
		}
		for _, tweaker := range meta.Tweakers {
			for _, value := range []string{"--tweakClass", tweaker} {
				meta.detail.Arguments.Game = append(meta.detail.Arguments.Game, minecraft.Argument{Value: minecraft.ArgumentValue{value}})
			}
		}
		if slices.Contains(meta.Traits, "FirstThreadOnMacOS") {
			meta.detail.Arguments.JVM = append(meta.detail.Arguments.JVM, minecraft.Argument{
				Rules: []minecraft.Rule{{Action: "allow", OS: &minecraft.OSRule{Name: "osx"}}},
				Value: minecraft.ArgumentValue{"-XstartOnFirstThread"},
			})
		}
		detail = minecraft.MergeVersion(detail, &meta.detail)
	}
	if detail.MainClass == "io.github.zekerzhayard.forgewrapper.installer.Main" {
		detail.Arguments.JVM = append(detail.Arguments.JVM, minecraft.Argument{Value: minecraft.ArgumentValue{"-Dforgewrapper.librariesDir=${library_directory}"}})
	}
	if err := detail.Validate(); err != nil {
		return "", nil, err
	}
	return filepath.Join(instance.InstanceDir, ".tomori"), detail, nil
}

func (s *Service) prismMetadata(ctx context.Context, shared, instanceDir string, component minecraft.PrismComponent) (*prismMetadata, error) {
	if err := fsutil.ValidateID(component.UID); err != nil {
		return nil, err
	}
	if err := fsutil.ValidateID(component.Version); err != nil {
		return nil, err
	}
	rel := filepath.Join("meta", component.UID, component.Version+".json")
	var raw []byte
	local := false
	if root, err := os.OpenRoot(instanceDir); err == nil {
		raw, err = fsutil.ReadRootFile(root, filepath.Join("patches", component.UID+".json"), 16<<20)
		_ = root.Close()
		if err == nil {
			local = true
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if !local {
		if root, err := os.OpenRoot(shared); err == nil {
			raw, err = fsutil.ReadRootFile(root, rel, 16<<20)
			_ = root.Close()
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	missing := len(raw) == 0
	if missing {
		var err error
		raw, err = s.http.Get(ctx, "https://meta.prismlauncher.org/v1/"+filepath.ToSlash(filepath.Join(component.UID, component.Version+".json")))
		if err != nil {
			return nil, err
		}
	}
	var meta prismMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return nil, err
	}
	if meta.UID == "" {
		meta.UID = component.UID
	}
	if meta.Version == "" {
		meta.Version = component.Version
	}
	if meta.MCVersion != "" {
		found := false
		for _, required := range meta.Requires {
			if required.UID == "net.minecraft" {
				found = true
				break
			}
		}
		if !found {
			meta.Requires = append(meta.Requires, struct{ UID, Equals, Suggests string }{UID: "net.minecraft", Equals: meta.MCVersion})
		}
	}
	if meta.FormatVersion == 0 {
		meta.FormatVersion = 1
	}
	if meta.FormatVersion != 1 || meta.UID != component.UID || meta.Version != component.Version {
		return nil, fmt.Errorf(locales.PrismComponentMetadataMismatch(), component.UID)
	}
	if err := json.Unmarshal(raw, &meta.detail); err != nil {
		return nil, err
	}
	meta.detail.Libraries = append(meta.detail.Libraries, meta.Libraries...)
	normalizePrismLibraries(meta.detail.Libraries)
	normalizePrismLibraries(meta.detail.MavenFiles)
	if missing {
		if err := fsutil.WriteAtomic(filepath.Join(shared, rel), raw); err != nil {
			return nil, err
		}
	}
	return &meta, nil
}

func prismSharedRoot(instance minecraft.Instance) string {
	for _, root := range prismRoots(instance) {
		for _, name := range []string{"prismlauncher.cfg", "multimc.cfg", filepath.Join("meta", "net.minecraft", instance.GameVersion+".json")} {
			if info, err := os.Stat(filepath.Join(root, name)); err == nil && info.Mode().IsRegular() {
				return root
			}
		}
	}
	if parent := filepath.Dir(instance.InstanceDir); strings.EqualFold(filepath.Base(parent), "instances") {
		return filepath.Dir(parent)
	}
	return instance.InstanceDir
}

func prismRoots(instance minecraft.Instance) []string {
	roots := []string{instance.GameDir, instance.InstanceDir, instance.SourceDir, filepath.Dir(instance.InstanceDir), filepath.Dir(filepath.Dir(instance.InstanceDir)), config.DefaultGameDirectory()}
	var result []string
	for _, root := range roots {
		if root != "" && !slices.Contains(result, root) {
			result = append(result, root)
		}
	}
	return result
}

func normalizePrismLibraries(libraries []minecraft.Library) {
	for i := range libraries {
		var rules []minecraft.Rule
		for _, rule := range libraries[i].Rules {
			if rule.OS == nil {
				rules = append(rules, rule)
				continue
			}
			osRule := *rule.OS
			rule.OS = &osRule
			name, arch, found := strings.Cut(osRule.Name, "-")
			if found {
				rule.OS.Name, rule.OS.Arch = name, arch
			} else if osRule.Name != "" && osRule.Arch == "" {
				// Bare OS names in Prism metadata cover both legacy x86
				// architectures. ARM requires an explicit [os]-[arch] rule.
				legacy := rule
				legacy.OS = &minecraft.OSRule{Name: osRule.Name, Version: osRule.Version, Arch: "x86"}
				rules = append(rules, legacy)
				rule.OS.Arch = "x86_64"
			}
			rules = append(rules, rule)
		}
		libraries[i].Rules = rules
	}
}
