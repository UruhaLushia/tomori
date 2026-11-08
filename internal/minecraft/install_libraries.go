package minecraft

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/network/download"
	"github.com/UruhaLushia/tomori/locales"
)

func (l Library) Artifact() (*FileRef, error) {
	if l.Downloads.Artifact != nil {
		ref := *l.Downloads.Artifact
		if ref.Path == "" {
			var err error
			ref.Path, err = MavenPath(l.Name)
			if err != nil {
				return nil, err
			}
		}
		return &ref, nil
	}
	// Older vanilla metadata only contains Maven coordinates.
	if len(l.Natives) > 0 {
		return nil, nil
	}
	path, err := MavenPath(l.Name)
	if err != nil {
		return nil, err
	}
	base := l.URL
	if base == "" {
		base = "https://libraries.minecraft.net/"
	}
	return &FileRef{Path: path, URL: strings.TrimRight(base, "/") + "/" + path, SHA1: l.SHA1, Size: l.Size}, nil
}

func MavenPath(coordinate string) (string, error) {
	coordinate, extension, hasExtension := strings.Cut(coordinate, "@")
	if !hasExtension {
		extension = "jar"
	}
	parts := strings.Split(coordinate, ":")
	if len(parts) < 3 || len(parts) > 4 {
		return "", fmt.Errorf(locales.InvalidDependencyCoordinateAlt(), coordinate)
	}
	for _, part := range append(parts, extension) {
		if err := fsutil.ValidateID(part); err != nil {
			return "", err
		}
	}
	name := parts[1] + "-" + parts[2]
	if len(parts) == 4 {
		name += "-" + parts[3]
	}
	path := strings.ReplaceAll(parts[0], ".", "/") + "/" + parts[1] + "/" + parts[2] + "/" + name + "." + extension
	if _, err := fsutil.SafePath(".", path); err != nil {
		return "", err
	}
	return path, nil
}

func (i *Installer) libraryTasks(detail *VersionDetail) (libs, natives []download.Task, err error) {
	task := func(ref FileRef) (download.Task, error) {
		path, err := fsutil.SafePath(i.LibrariesDir(), ref.Path)
		if err != nil {
			return download.Task{}, err
		}
		if ref.URL == "" && !fsutil.Complete(path, ref.SHA1, ref.Size) {
			return download.Task{}, fmt.Errorf(locales.DependencyDownloadUrlMissing(), ref.Path)
		}
		return download.Task{URL: ref.URL, Path: path, SHA1: ref.SHA1, Size: ref.Size}, nil
	}
	for _, l := range slices.Concat(detail.LibrariesFor(i.Env), detail.MavenFiles) {
		if !l.Applies(i.Env) {
			continue
		}
		a, err := l.Artifact()
		if err != nil {
			return nil, nil, err
		}
		if a != nil {
			t, err := task(*a)
			if err != nil {
				return nil, nil, err
			}
			libs = append(libs, t)
		}
		if len(l.Natives) == 0 {
			continue
		}
		classifier := NativeClassifier(l.Natives, i.Env)
		if classifier == "" {
			continue
		}
		ref, ok := l.Downloads.Classifiers[classifier]
		if !ok || ref.Path == "" {
			return nil, nil, fmt.Errorf(locales.IsMissingNativeLibrary(), l.Name, classifier)
		}
		t, err := task(ref)
		if err != nil {
			return nil, nil, err
		}
		natives = append(natives, t)
	}
	return libs, natives, nil
}

func (i *Installer) extractNatives(ctx context.Context, detail *VersionDetail) error {
	type nativeJar struct {
		path    string
		exclude []string
	}
	var jars []nativeJar
	for _, l := range detail.LibrariesFor(i.Env) {
		if len(l.Natives) == 0 {
			continue
		}
		classifier := NativeClassifier(l.Natives, i.Env)
		ref, ok := l.Downloads.Classifiers[classifier]
		if !ok || ref.Path == "" {
			continue
		}
		exclude := []string{"META-INF/"}
		if l.Extract != nil && len(l.Extract.Exclude) > 0 {
			exclude = l.Extract.Exclude
		}
		jars = append(jars, nativeJar{
			path:    filepath.Join(i.LibrariesDir(), filepath.FromSlash(ref.Path)),
			exclude: exclude,
		})
	}
	dir := i.NativesDir(detail.ID)
	stage, err := os.MkdirTemp(i.VersionDir(detail.ID), ".tomori-natives-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for _, jar := range jars {
		if err := extractZip(ctx, jar.path, stage, jar.exclude); err != nil {
			return fmt.Errorf(locales.ExtractionFailed(), filepath.Base(jar.path), err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Publish a fresh extraction only after every jar is usable. Reusing the
	// old directory can retain libraries removed by a loader or Java change.
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return os.Rename(stage, dir)
}
