package minecraft

import (
	"slices"
	"strings"
)

// MergeVersion materializes an inherited loader profile as a self-contained
// version, so launch and repair do not depend on the exporting launcher.
func MergeVersion(base, child *VersionDetail) *VersionDetail {
	v := *base
	v.Jar = base.ClientID()
	v.InheritsFrom = ""
	v.Patches = nil
	v.ComplianceLevel = max(base.ComplianceLevel, child.ComplianceLevel)
	v.MinimumLauncherVersion = max(base.MinimumLauncherVersion, child.MinimumLauncherVersion)
	if v.MinecraftVersion == "" {
		v.MinecraftVersion = base.ID
	}
	if child.MinecraftVersion != "" {
		v.MinecraftVersion = child.MinecraftVersion
	}
	if child.ID != "" {
		v.ID = child.ID
	}
	if child.Jar != "" {
		v.Jar = child.Jar
	}
	if child.MainClass != "" {
		v.MainClass = child.MainClass
	}
	if child.Type != "" {
		v.Type = child.Type
	}
	if child.JavaVersion != nil {
		v.JavaVersion = child.JavaVersion
	}
	v.CompatibleJavaMajors = slices.Concat(base.CompatibleJavaMajors, child.CompatibleJavaMajors)
	if child.AssetIndex.ID != "" {
		v.AssetIndex = child.AssetIndex
	}
	if child.Assets != "" {
		v.Assets = child.Assets
	}
	if child.Downloads.Client.URL != "" {
		v.Downloads.Client = child.Downloads.Client
	}
	if child.Downloads.ClientMappings.URL != "" {
		v.Downloads.ClientMappings = child.Downloads.ClientMappings
	}
	if child.Logging != nil {
		v.Logging = child.Logging
	}
	if !child.ReleaseTime.IsZero() {
		v.ReleaseTime = child.ReleaseTime
	}
	if !child.Time.IsZero() {
		v.Time = child.Time
	}
	if child.MinecraftArguments != "" {
		v.MinecraftArguments = child.MinecraftArguments
		v.Arguments.Game = nil
	}
	if len(v.Arguments.Game) == 0 && v.MinecraftArguments != "" && len(child.Arguments.Game) > 0 {
		// Older profiles use the legacy string for their entire game argument list.
		// Loader-only arguments cannot replace that list; legacy loaders provide
		// their own minecraftArguments above.
		v.Arguments.Game = legacyArguments(v.MinecraftArguments)
	}
	v.Arguments.Game = append(slices.Clone(v.Arguments.Game), child.Arguments.Game...)
	if len(v.Arguments.JVM) == 0 && len(child.Arguments.JVM) > 0 && base.MainClass != "" {
		for _, value := range []string{"-Djava.library.path=${natives_directory}", "-cp", "${classpath}"} {
			v.Arguments.JVM = append(v.Arguments.JVM, Argument{Value: ArgumentValue{value}})
		}
	}
	v.Arguments.JVM = append(slices.Clone(v.Arguments.JVM), child.Arguments.JVM...)
	v.MavenFiles = append(slices.Clone(base.MavenFiles), child.MavenFiles...)
	// Preserve variants until the Java platform is known. The same native
	// coordinate can supply different OS classifiers without explicit rules.
	v.Libraries = slices.Concat(child.Libraries, base.Libraries)
	isLwjgl3 := func(library Library) bool { return strings.HasPrefix(library.Name, "org.lwjgl:") }
	isLwjgl2 := func(library Library) bool { return strings.HasPrefix(library.Name, "org.lwjgl.lwjgl:") }
	if slices.ContainsFunc(v.Libraries, isLwjgl3) {
		v.Libraries = slices.DeleteFunc(v.Libraries, isLwjgl2)
	}
	return &v
}

func libraryKey(library Library) string {
	key := libraryCoordinate(library.Name)
	if len(library.Natives) > 0 {
		key += "#natives"
	}
	return key
}

// LibrariesFor selects the first applicable variant after child-first merging.
// Filtering rules before deduplication keeps an override for Linux from
// accidentally deleting the base dependency needed on Windows or macOS.
func (d *VersionDetail) LibrariesFor(env Environment) []Library {
	result := make([]Library, 0, len(d.Libraries))
	seen := make(map[string]bool)
	for _, library := range d.Libraries {
		if !library.Applies(env) || len(library.Natives) > 0 && NativeClassifier(library.Natives, env) == "" {
			continue
		}
		key := libraryKey(library)
		if !seen[key] {
			seen[key] = true
			result = append(result, library)
		}
	}
	return result
}

func libraryCoordinate(name string) string {
	coordinate, extension, hasExtension := strings.Cut(name, "@")
	if !hasExtension {
		extension = "jar"
	}
	parts := strings.Split(coordinate, ":")
	if len(parts) < 3 {
		return name
	}
	// Maven classifier is part of the identity, while the version is the
	// replaceable part of an inherited dependency.
	classifier := ""
	if len(parts) > 3 {
		classifier = parts[3]
	}
	return parts[0] + ":" + parts[1] + ":" + classifier + "@" + extension
}

func legacyArguments(raw string) []Argument {
	// Mojang legacy metadata uses whitespace-separated flags and placeholders.
	var args []Argument
	for value := range strings.FieldsSeq(raw) {
		args = append(args, Argument{Value: ArgumentValue{value}})
	}
	return args
}
