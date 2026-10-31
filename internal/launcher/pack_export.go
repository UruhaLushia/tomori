package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type PackExport struct {
	Name, Version, Description, Format, Directory, Filename string
	Worlds, Screenshots, Logs, Personal, References         bool
}

func (r PackExport) Extension() string {
	if r.Format == "prism" || r.Format == "curseforge" || r.Format == "mcbbs" {
		return ".zip"
	}
	return ".mrpack"
}

type PackExportResult struct {
	Path                string
	References, Bundled int
	Offline             bool
}

func ExportFilename(name string) string {
	var out strings.Builder
	for _, r := range name {
		if out.Len()+utf8.RuneLen(r) > 120 {
			break
		}
		if r < 32 || r == 127 || strings.ContainsRune("/\\:\x00<>\"|?*", r) {
			out.WriteByte('-')
		} else {
			out.WriteRune(r)
		}
	}
	value := strings.Trim(out.String(), " .")
	if fsutil.ValidateID(value) != nil {
		return "modpack"
	}
	return value
}

func (s *Service) ExportPack(ctx context.Context, instance minecraft.Instance, request PackExport, progress func(minecraft.Progress)) (result PackExportResult, err error) {
	if request.Format != "mrpack" && request.Format != "prism" && request.Format != "curseforge" && request.Format != "mcbbs" {
		return result, errors.New(locales.UnsupportedExportFormat())
	}
	for _, value := range []string{request.Name, request.Version} {
		if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || utf8.RuneCountInString(value) > 128 || strings.ContainsFunc(value, unicode.IsControl) {
			return result, errors.New(locales.NameAndVersionMustContain1128CharactersWithoutControlCharacters())
		}
	}
	if !utf8.ValidString(request.Description) || utf8.RuneCountInString(request.Description) > 4096 || strings.ContainsRune(request.Description, 0) {
		return result, errors.New(locales.ModpackDescriptionIsTooLongOrInvalid())
	}
	if err := fsutil.ValidateID(request.Filename); err != nil {
		return result, err
	}
	if len(request.Filename) > 220 {
		return result, errors.New(locales.FileNameIsTooLong())
	}
	s = s.forInstance(instance)
	content := instance.ContentDirectory(s.cfg.Game.Isolate)
	release, err := s.lockRoot(ctx, content, progress)
	if err != nil {
		return result, err
	}
	defer release()
	components, err := minecraft.InspectExportComponents(ctx, instance)
	if err != nil {
		return result, err
	}
	if (request.Format != "prism" || instance.Format != minecraft.InstancePrism) && components.Reason != "" {
		return result, fmt.Errorf(locales.ThisFormatDoesNotSupportTheInstanceComponents(), components.Reason)
	}
	directory, err := fsutil.DirectoryPath(request.Directory)
	if err != nil {
		return result, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return result, err
	}
	for _, path := range []string{content, instance.InstanceDir} {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			return result, err
		}
		if pathWithin(real, directory) {
			return result, errors.New(locales.TheExportFolderCannotBeInsideTheSourceInstance())
		}
	}
	destination, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer destination.Close()
	filename := request.Filename + request.Extension()
	if _, err := destination.Lstat(filename); err == nil {
		return result, errors.New(locales.AFileAlreadyExistsWithThisNameChooseAnotherName())
	} else if !errors.Is(err, os.ErrNotExist) {
		return result, err
	}
	source, err := os.OpenRoot(content)
	if err != nil {
		return result, err
	}
	defer source.Close()
	include := func(path string, _ bool) bool {
		top, _, _ := strings.Cut(strings.ToLower(filepath.ToSlash(path)), "/")
		switch top {
		case "versions", "libraries", "assets", "pack.json", "modrinth.index.json", "manifest.json", "modpack.json", "mcbbs.packmeta", "mmc-pack.json", "instance.cfg", ".fabric", ".quilt", ".forge", ".neoforge", ".ds_store", "thumbs.db":
			return false
		}
		return copyContent(path, instance, InstanceCopy{Worlds: request.Worlds, Screenshots: request.Screenshots, Logs: request.Logs, Personal: request.Personal})
	}
	var references map[string]exportReference
	if (request.Format == "mrpack" || request.Format == "curseforge") && request.References {
		if progress != nil {
			progress(minecraft.Progress{Stage: "export-identify"})
		}
		if request.Format == "curseforge" {
			references, result.Offline, err = s.exportCurseReferences(ctx, source, include, progress)
		} else {
			references, result.Offline, err = s.exportReferences(ctx, source, include, progress)
		}
		if err != nil {
			return result, err
		}
	}
	temp := ".tomori-export-" + rand.Text()
	out, err := destination.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	tempInfo, err := out.Stat()
	if err != nil {
		_ = out.Close()
		return result, err
	}
	defer func() {
		_ = out.Close()
		current, e := destination.Lstat(temp)
		if e == nil && os.SameFile(tempInfo, current) {
			_ = destination.Remove(temp)
		}
	}()
	z := fsutil.NewZipExporter(ctx, out, s.client.FilePermit, func(p fsutil.CopyProgress) {
		if progress != nil {
			progress(minecraft.Progress{Stage: "export", Done: p.Bytes, FilesDone: p.Files})
		}
	})
	closed := false
	defer func() {
		if !closed {
			_ = z.Close()
		}
	}()
	prefix := "overrides/"
	if request.Format == "prism" {
		prefix = "minecraft/"
	}
	var mcbbs exportMCBBSFiles
	err = z.Tree(source, prefix, include, func(path string, info os.FileInfo) (bool, error) {
		if request.Format == "mcbbs" {
			hash, err := z.FileSHA1(source, filepath.FromSlash(path), prefix+path, info)
			if err != nil {
				return false, err
			}
			return true, mcbbs.add(path, hash)
		}
		ref, found := references[path]
		if !found {
			return false, nil
		}
		if !sameExportFile(ref.info, info) {
			return false, errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		result.References++
		return true, nil
	})
	if err != nil {
		return result, err
	}
	result.Bundled = z.Progress().Files
	if request.Format == "mrpack" {
		if err := writeExportIndex(z, request, components, references); err != nil {
			return result, err
		}
	} else if request.Format == "curseforge" {
		if err := writeExportCurse(z, request, components, references); err != nil {
			return result, err
		}
	} else if request.Format == "mcbbs" {
		if err := writeExportMCBBS(z, request, components, mcbbs.files); err != nil {
			return result, err
		}
	} else if err := s.writeExportPrism(ctx, z, instance, request, components); err != nil {
		return result, err
	}
	if icon, e := minecraft.LoadInstanceIcon(ctx, instance); e == nil && icon != nil {
		var raw bytes.Buffer
		if err := png.Encode(&raw, icon); err != nil {
			return result, err
		}
		if err := z.Bytes("icon.png", raw.Bytes()); err != nil {
			return result, err
		}
	}
	after, err := minecraft.InspectExportComponents(ctx, instance)
	if err != nil {
		return result, err
	}
	if after.Fingerprint != components.Fingerprint || after.ProfileFingerprint != components.ProfileFingerprint {
		return result, errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	if err := verifyExportReferences(ctx, source, references); err != nil {
		return result, err
	}
	closed = true
	if err := z.Close(); err != nil {
		return result, err
	}
	if err := out.Sync(); err != nil {
		return result, err
	}
	publishInfo, err := out.Stat()
	if err != nil {
		return result, err
	}
	if err := out.Close(); err != nil {
		return result, err
	}
	publishRelease, err := s.client.FilePermit(ctx)
	if err != nil {
		return result, err
	}
	err = fsutil.PublishRootExclusive(ctx, destination, temp, filename, publishInfo)
	publishRelease()
	if err != nil {
		return result, err
	}
	result.Path = filepath.Join(directory, filename)
	return result, nil
}

func sameExportFile(a, b os.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

func (s *Service) writeExportPrism(ctx context.Context, z *fsutil.ZipExporter, instance minecraft.Instance, request PackExport, components minecraft.InstanceComponents) error {
	if instance.Format == minecraft.InstancePrism {
		root, err := os.OpenRoot(instance.InstanceDir)
		if err != nil {
			return err
		}
		defer root.Close()
		if err := z.File(root, "mmc-pack.json", "mmc-pack.json", nil); err != nil {
			return err
		}
		if err := z.Tree(root, "", func(path string, _ bool) bool {
			top, _, _ := strings.Cut(filepath.ToSlash(path), "/")
			return top == "patches" || top == "jarmods" || top == "libraries"
		}, nil); err != nil {
			return err
		}
	} else {
		pack := minecraft.PrismPack{FormatVersion: 1, Components: []minecraft.PrismComponent{{UID: "net.minecraft", Version: components.Game}}}
		if components.Loader != "" {
			pack.Components = append(pack.Components, minecraft.PrismComponent{UID: minecraft.LoaderUID(components.Loader), Version: components.LoaderVersion})
		}
		raw, err := json.Marshal(pack)
		if err != nil {
			return err
		}
		if err := z.Bytes("mmc-pack.json", raw); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	meta, err := json.Marshal(minecraft.PrismMetadata{Description: request.Description, PackFormat: "Prism / MultiMC", PackVersion: request.Version})
	if err != nil {
		return err
	}
	if err := z.Bytes(minecraft.PrismMetadataFile, meta); err != nil {
		return err
	}
	return z.Bytes("instance.cfg", []byte("[General]\nInstanceType=OneSix\nname="+strconv.Quote(request.Name)+"\nnotes="+strconv.Quote(request.Description)+"\niconKey=icon\n"))
}
