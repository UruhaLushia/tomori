package java

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/network/download"
	httpclient "github.com/UruhaLushia/tomori/internal/network/http"
	"github.com/UruhaLushia/tomori/locales"
)

const foojayPackagesURL = "https://api.foojay.io/disco/v3.0/packages"

type foojayPackage struct {
	ID          string `json:"id"`
	ArchiveType string `json:"archive_type"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
}

type foojayPackagesResponse struct {
	Result []foojayPackage `json:"result"`
}

type foojayPackageInfo struct {
	Filename     string `json:"filename"`
	Download     string `json:"direct_download_uri"`
	Checksum     string `json:"checksum"`
	ChecksumType string `json:"checksum_type"`
}

type foojayPackageInfoResponse struct {
	Result []foojayPackageInfo `json:"result"`
}

func downloadFoojayRuntime(ctx context.Context, client *httpclient.Client, f *download.Client, request DownloadRequest, dir string, progress func(minecraft.Progress)) (Runtime, error) {
	targetPlatform, operatingSystem, architecture, archiveType, ok := zuluTarget()
	if !ok {
		return Runtime{}, errors.New(locales.NoOfficialJavaRuntimeForThisPlatformChooseAPathManually())
	}
	query := url.Values{
		"distro":                {string(request.Distribution)},
		"version":               {strconv.Itoa(request.Major)},
		"package_type":          {string(request.PackageType)},
		"operating_system":      {operatingSystem},
		"architecture":          {architecture},
		"archive_type":          {archiveType},
		"release_status":        {"ga"},
		"latest":                {"available"},
		"directly_downloadable": {"true"},
	}
	var packages foojayPackagesResponse
	if err := client.GetJSON(ctx, foojayPackagesURL+"?"+query.Encode(), &packages); err != nil {
		return Runtime{}, err
	}
	if len(packages.Result) == 0 {
		return Runtime{}, fmt.Errorf(locales.NoOfficialRuntimeForChooseJavaManually(), operatingSystem, string(request.Distribution)+" "+strconv.Itoa(request.Major))
	}
	selected := packages.Result[0]
	if selected.ID == "" || selected.Size <= 0 || selected.ArchiveType != archiveType {
		return Runtime{}, errors.New(locales.RuntimeFileMetadataIncomplete())
	}
	var info foojayPackageInfoResponse
	if err := client.GetJSON(ctx, "https://api.foojay.io/disco/v3.0/ids/"+url.PathEscape(selected.ID), &info); err != nil {
		return Runtime{}, err
	}
	if len(info.Result) == 0 || info.Result[0].Download == "" || !strings.EqualFold(info.Result[0].ChecksumType, "sha256") {
		return Runtime{}, errors.New(locales.RuntimeFileMetadataIncomplete())
	}
	ref := info.Result[0]
	filename := ref.Filename
	if filename == "" {
		filename = selected.Filename
	}
	if filename == "" {
		return Runtime{}, errors.New(locales.RuntimeFileMetadataIncomplete())
	}
	if filepath.Base(filename) != filename || strings.ContainsAny(filename, `/\\`) || fsutil.ValidateID(filename) != nil || len(ref.Checksum) != sha256.Size*2 {
		return Runtime{}, errors.New(locales.RuntimeFileMetadataIncomplete())
	}
	if progress != nil {
		progress(minecraft.Progress{Stage: "java"})
	}
	component := fmt.Sprintf("%s-%s-%d", request.Distribution, request.PackageType, request.Major)
	if err := fsutil.ValidateID(component); err != nil {
		return Runtime{}, err
	}
	parent := filepath.Join(dir, component)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Runtime{}, err
	}
	stage, err := os.MkdirTemp(parent, ".tomori-zulu-*")
	if err != nil {
		return Runtime{}, err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	archivePath := filepath.Join(stage, filename)
	if err := f.Fetch(ctx, download.Task{URL: ref.Download, Path: archivePath, Size: selected.Size}); err != nil {
		return Runtime{}, err
	}
	if err := verifySHA256(archivePath, ref.Checksum); err != nil {
		return Runtime{}, err
	}
	extracted := filepath.Join(stage, "files")
	if err := os.MkdirAll(extracted, 0o755); err != nil {
		return Runtime{}, err
	}
	if err := extractJavaArchive(ctx, archivePath, extracted); err != nil {
		return Runtime{}, err
	}
	root, executable, err := findJavaRoot(ctx, extracted)
	if err != nil {
		return Runtime{}, err
	}
	if err := fsutil.WriteAtomic(filepath.Join(root, ".complete"), []byte(component)); err != nil {
		return Runtime{}, err
	}
	target := filepath.Join(parent, targetPlatform)
	if err := publishRuntime(ctx, root, target); err != nil {
		return Runtime{}, err
	}
	relative, err := filepath.Rel(root, executable)
	if err != nil {
		return Runtime{}, err
	}
	path := filepath.Join(target, relative)
	result, err := inspectCached(ctx, path)
	if err != nil {
		return Runtime{}, err
	}
	result.Path = path
	return result, nil
}

func zuluTarget() (platform, operatingSystem, architecture, archiveType string, ok bool) {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64":
		return "linux", "linux", "x64", "tar.gz", true
	case "linux/386":
		return "linux-i386", "linux", "x86", "tar.gz", true
	case "windows/amd64":
		return "windows-x64", "windows", "x64", "zip", true
	case "windows/386":
		return "windows-x86", "windows", "x86", "zip", true
	case "windows/arm64":
		return "windows-arm64", "windows", "aarch64", "zip", true
	case "darwin/amd64":
		return "mac-os", "macos", "x64", "tar.gz", true
	case "darwin/arm64":
		return "mac-os-arm64", "macos", "aarch64", "tar.gz", true
	default:
		return "", "", "", "", false
	}
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), expected) {
		return fmt.Errorf(locales.VerificationFailed(), filepath.Base(path))
	}
	return nil
}

func extractJavaArchive(ctx context.Context, archivePath, destination string) error {
	if strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		return extractJavaZip(ctx, archivePath, destination)
	}
	return extractJavaTarGz(ctx, archivePath, destination)
}

func extractJavaZip(ctx context.Context, archivePath, destination string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = archive.Close() }()
	var total int64
	for _, entry := range archive.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := extractJavaZipEntry(entry, destination, &total); err != nil {
			return err
		}
	}
	return nil
}

func extractJavaZipEntry(entry *zip.File, destination string, total *int64) error {
	name := strings.TrimSuffix(strings.ReplaceAll(entry.Name, "\\", "/"), "/")
	if name == "" {
		return nil
	}
	path, err := archivePath(destination, name)
	if err != nil {
		return err
	}
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(path, 0o755)
	}
	if entry.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), name)
	}
	if !entry.FileInfo().Mode().IsRegular() {
		return fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), name)
	}
	if entry.UncompressedSize64 > 512<<20 || *total > 8<<30-int64(entry.UncompressedSize64) {
		return errors.New(locales.ArchiveExtractionSizeOrFileCountExceedsLimit())
	}
	*total += int64(entry.UncompressedSize64)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	in, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(in, int64(entry.UncompressedSize64)+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != int64(entry.UncompressedSize64) {
		return errors.New(locales.RuntimeFileMetadataIncomplete())
	}
	return os.Chmod(path, entry.Mode().Perm())
}

func extractJavaTarGz(ctx context.Context, archiveFile, destination string) error {
	file, err := os.Open(archiveFile)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer func() { _ = gzipReader.Close() }()
	reader := tar.NewReader(gzipReader)
	var total int64
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(strings.ReplaceAll(header.Name, "\\", "/"), "/")
		if name == "" {
			continue
		}
		path, err := archivePath(destination, name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := extractJavaSymlink(destination, name, header.Linkname); err != nil {
				return err
			}
		case tar.TypeReg:
			if header.Size < 0 || header.Size > 512<<20 || total > 8<<30-header.Size {
				return errors.New(locales.ArchiveExtractionSizeOrFileCountExceedsLimit())
			}
			total += header.Size
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(out, io.LimitReader(reader, header.Size+1))
			closeErr := out.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
			if written != header.Size {
				return errors.New(locales.RuntimeFileMetadataIncomplete())
			}
			if err := os.Chmod(path, os.FileMode(header.Mode).Perm()); err != nil {
				return err
			}
		default:
			return fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), name)
		}
	}
}

func extractJavaSymlink(destination, name, target string) error {
	if target == "" || filepath.IsAbs(filepath.FromSlash(target)) || strings.ContainsAny(target, `:\`) {
		return fmt.Errorf(locales.InvalidJavaLink(), name)
	}
	if _, err := fsutil.SafePath(destination, filepath.Join(filepath.Dir(name), target)); err != nil {
		return fmt.Errorf(locales.InvalidJavaLink(), name)
	}
	path, err := fsutil.SafePath(destination, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.Symlink(filepath.FromSlash(target), path)
}

func archivePath(destination, name string) (string, error) {
	path, err := fsutil.SafePath(destination, name)
	if err != nil {
		return "", err
	}
	for part := range strings.SplitSeq(name, "/") {
		if err := fsutil.ValidateID(part); err != nil {
			return "", err
		}
	}
	return path, nil
}

func findJavaRoot(ctx context.Context, extracted string) (string, string, error) {
	var root, executable string
	err := filepath.WalkDir(extracted, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Base(path) != javaBinary() {
			return nil
		}
		candidate := filepath.Dir(filepath.Dir(path))
		if _, err := os.Stat(filepath.Join(candidate, "release")); err != nil {
			return nil
		}
		root, executable = candidate, path
		return filepath.SkipAll
	})
	if err != nil {
		return "", "", err
	}
	if root == "" {
		return "", "", errors.New(locales.DownloadedJavaCannotRun())
	}
	return root, executable, nil
}
