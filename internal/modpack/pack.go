package modpack

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

type Loader struct{ ID, Version string }

func (l Loader) String() string {
	name := map[string]string{"fabric": "Fabric", "quilt": "Quilt", "forge": "Forge", "neoforge": "NeoForge"}[l.ID]
	if name == "" {
		name = l.ID
	}
	return strings.TrimSpace(name + " " + l.Version)
}

type File struct {
	Path, SHA1, SHA512, Website string
	Size                        int64
	URLs                        []string
	Optional                    bool
	ProjectID, FileID           int64
}

type Info struct {
	Format, Name, Version, Author, Description, GameVersion string
	Loaders                                                 []Loader
	Files                                                   []File
	Warnings                                                []string
	ExpandedSize                                            uint64
}

func (f File) SelectionKey() string {
	if f.Path != "" {
		return f.Path
	}
	return fmt.Sprintf("%d:%d", f.ProjectID, f.FileID)
}

func (i Info) WithOptional(selected map[string]bool) Info {
	if selected == nil {
		return i
	}
	files := make([]File, 0, len(i.Files))
	for _, file := range i.Files {
		if !file.Optional || selected[file.SelectionKey()] {
			files = append(files, file)
		}
	}
	i.Files = files
	return i
}

func (i Info) LoaderLabel() string {
	var labels []string
	for _, l := range i.Loaders {
		labels = append(labels, l.String())
	}
	return strings.Join(labels, " · ")
}

type Archive struct {
	Info      Info
	zip       *zip.Reader
	file      *os.File
	stamp     os.FileInfo
	name      string
	identity  *ArchiveIdentity
	entries   map[string]*zip.File
	prefix    string
	overrides []string
	Profile   *minecraft.VersionDetail
	temporary string
	bundled   map[string]string
}

func Open(ctx context.Context, filename string) (_ *Archive, err error) {
	return open(ctx, filename, 0)
}

func open(ctx context.Context, filename string, depth int) (_ *Archive, err error) {
	if depth > 3 {
		return nil, errors.New(locales.ModpackNestingExceedsLimit())
	}
	file, err := fsutil.OpenRegular(filename)
	if err != nil {
		return nil, fmt.Errorf(locales.CannotReadModpackUseZipOrMrpack(), err)
	}
	a := &Archive{file: file, name: filepath.Base(filename), entries: make(map[string]*zip.File)}
	defer func() {
		if err != nil {
			_ = a.Close()
		}
	}()
	if a.stamp, err = file.Stat(); err != nil {
		return nil, err
	}
	z, err := fsutil.NewZipReader(ctx, file, a.stamp.Size())
	if err != nil {
		return nil, fmt.Errorf(locales.CannotReadModpackUseZipOrMrpack(), err)
	}
	a.zip = z
	return a.inspect(ctx, depth, true)
}

var ErrPreviewDeferred = errors.New("archive preview requires extraction")

func Preview(ctx context.Context, source io.ReaderAt, size int64, name string) (Info, error) {
	z, err := fsutil.NewZipReader(ctx, source, size)
	if err != nil {
		return Info{}, err
	}
	a := &Archive{zip: z, name: name, entries: make(map[string]*zip.File)}
	_, err = a.inspect(ctx, 0, false)
	return a.Info, err
}

func (a *Archive) inspect(ctx context.Context, depth int, nested bool) (*Archive, error) {
	var err error
	for _, f := range a.zip.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(strings.ReplaceAll(f.Name, "\\", "/"), "/")
		if name == "" {
			continue
		}
		if _, err := fsutil.SafePath(".", name); err != nil {
			return nil, err
		}
		if f.Mode()&os.ModeSymlink != 0 || (!f.FileInfo().IsDir() && !f.Mode().IsRegular()) {
			return nil, fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), name)
		}
		if _, exists := a.entries[name]; exists {
			return nil, fmt.Errorf(locales.DuplicateArchivePath(), name)
		}
		a.entries[name] = f
	}
	// Prefer manifests at the shallowest common level. A wrapper directory is allowed.
	var candidates []string
	for name := range a.entries {
		if slices.Contains([]string{"mcbbs.packmeta", "mmc-pack.json", "modrinth.index.json", "manifest.json", "modpack.json"}, path.Base(name)) {
			candidates = append(candidates, name)
		}
	}
	slices.SortFunc(candidates, func(a, b string) int {
		if n := strings.Count(a, "/") - strings.Count(b, "/"); n != 0 {
			return n
		}
		order := []string{"mcbbs.packmeta", "mmc-pack.json", "modrinth.index.json", "manifest.json", "modpack.json"}
		return slices.Index(order, path.Base(a)) - slices.Index(order, path.Base(b))
	})
	if len(candidates) > 0 {
		manifest := candidates[0]
		a.prefix = strings.TrimSuffix(manifest, path.Base(manifest))
		for _, other := range candidates[1:] {
			if strings.Count(other, "/") == strings.Count(manifest, "/") && strings.TrimSuffix(other, path.Base(other)) != a.prefix {
				return nil, errors.New(locales.ArchiveContainsMultipleModpacksImportSeparately())
			}
		}
		switch path.Base(manifest) {
		case "modrinth.index.json":
			err = a.readModrinth(ctx, manifest)
		case "mmc-pack.json":
			err = a.readPrism(ctx, manifest)
		case "mcbbs.packmeta":
			err = a.readMCBBS(ctx, manifest)
		case "manifest.json":
			var probe struct {
				Addons json.RawMessage `json:"addons"`
			}
			if err = a.readJSON(ctx, manifest, &probe); err == nil {
				if len(probe.Addons) > 0 {
					err = a.readMCBBS(ctx, manifest)
				} else {
					err = a.readCurseForge(ctx, manifest)
				}
			}
		case "modpack.json":
			err = a.readHMCL(ctx, manifest)
		}
	} else {
		var embedded []string
		for name := range a.entries {
			if path.Base(name) == "modpack.zip" || path.Base(name) == "modpack.mrpack" {
				embedded = append(embedded, name)
			}
		}
		if len(embedded) > 1 {
			return nil, errors.New(locales.ArchiveContainsMultipleEmbeddedModpacksImportSeparately())
		}
		for name, f := range a.entries {
			if path.Base(name) != "modpack.zip" && path.Base(name) != "modpack.mrpack" {
				continue
			}
			if !nested {
				return nil, ErrPreviewDeferred
			}
			if f.UncompressedSize64 > 4<<30 {
				return nil, errors.New(locales.EmbeddedModpackTooLarge())
			}
			in, e := f.Open()
			if e != nil {
				return nil, e
			}
			tmp, e := os.CreateTemp("", "tomori-modpack-*.zip")
			if e != nil {
				_ = in.Close()
				return nil, e
			}
			n, copyErr := io.Copy(tmp, io.LimitReader(fsutil.ContextReader(ctx, in), int64(f.UncompressedSize64)+1))
			e = copyErr
			if e == nil && n != int64(f.UncompressedSize64) {
				e = errors.New(locales.EmbeddedModpackTooLarge())
			}
			_ = in.Close()
			if closeErr := tmp.Close(); e == nil {
				e = closeErr
			}
			if e != nil {
				_ = os.Remove(tmp.Name())
				return nil, e
			}
			nested, e := open(ctx, tmp.Name(), depth+1)
			if e != nil {
				_ = os.Remove(tmp.Name())
				return nil, e
			}
			identity, e := a.Identity(ctx)
			if e != nil {
				_ = nested.Close()
				_ = os.Remove(tmp.Name())
				return nil, e
			}
			nested.identity = &identity
			// Each nested reader owns its backing file until Close.
			oldTemp := nested.temporary
			if oldTemp != "" {
				_ = os.Remove(tmp.Name())
			} else {
				nested.temporary = tmp.Name()
			}
			_ = a.Close()
			return nested, nil
		}
		err = a.readMinecraft(ctx)
	}
	if err != nil {
		return nil, err
	}
	if a.Info.Name == "" {
		a.Info.Name = strings.TrimSuffix(a.name, filepath.Ext(a.name))
	}
	if err = fsutil.ValidateID(a.Info.GameVersion); err != nil {
		return nil, fmt.Errorf(locales.MissingValidGameVersion(), err)
	}
	for _, l := range a.Info.Loaders {
		if err = fsutil.ValidateID(l.Version); err != nil {
			return nil, err
		}
		if !slices.Contains([]string{"forge", "neoforge", "fabric", "quilt"}, l.ID) {
			a.Info.Warnings = append(a.Info.Warnings, locales.UnsupportedLoader()+l.String())
		}
	}
	if len(a.Info.Loaders) > 1 {
		a.Info.Warnings = append(a.Info.Warnings, locales.InstallingMultipleLoadersIsUnsupported())
	}
	for _, f := range a.Info.Files {
		if f.Path != "" {
			if err = validContentPath(f.Path); err != nil {
				return nil, err
			}
		}
	}
	a.Info.ExpandedSize, err = a.expandedSize(ctx)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (a *Archive) Close() error {
	err := a.file.Close()
	if a.temporary != "" {
		_ = os.Remove(a.temporary)
	}
	return err
}

var archiveBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

func (a *Archive) Verify() error {
	after, err := a.file.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(a.stamp, after) || a.stamp.Size() != after.Size() || !a.stamp.ModTime().Equal(after.ModTime()) {
		return errors.New(locales.TheModpackChangedSelectItAgain())
	}
	return nil
}

func (a *Archive) Identity(ctx context.Context) (ArchiveIdentity, error) {
	if err := ctx.Err(); err != nil {
		return ArchiveIdentity{}, err
	}
	if err := a.Verify(); err != nil {
		return ArchiveIdentity{}, err
	}
	if a.identity != nil {
		return *a.identity, nil
	}
	h := sha512.New()
	buffer := archiveBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(h, fsutil.ContextReader(ctx, io.NewSectionReader(a.file, 0, a.stamp.Size()+1)), *buffer)
	archiveBuffers.Put(buffer)
	if err != nil {
		return ArchiveIdentity{}, err
	}
	if err := a.Verify(); err != nil || n != a.stamp.Size() {
		if err != nil {
			return ArchiveIdentity{}, err
		}
		return ArchiveIdentity{}, errors.New(locales.TheModpackChangedSelectItAgain())
	}
	identity := ArchiveIdentity{Name: a.name, SHA512: hex.EncodeToString(h.Sum(nil)), Size: n}
	a.identity = &identity
	return identity, nil
}

func (a *Archive) VerifySource(ctx context.Context, size int64, sum, sum512 string) error {
	identity, err := a.Identity(ctx)
	if err != nil {
		return err
	}
	if size > 0 && identity.Size != size || sum512 != "" && !strings.EqualFold(identity.SHA512, sum512) {
		return errors.New(locales.TheSelectedArchiveDoesNotMatchThisVersion())
	}
	if sum != "" {
		h := sha1.New()
		buffer := archiveBuffers.Get().(*[]byte)
		n, err := io.CopyBuffer(h, fsutil.ContextReader(ctx, io.NewSectionReader(a.file, 0, a.stamp.Size()+1)), *buffer)
		archiveBuffers.Put(buffer)
		if err != nil {
			return err
		}
		if n != a.stamp.Size() || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
			return errors.New(locales.TheSelectedArchiveDoesNotMatchThisVersion())
		}
	}
	return a.Verify()
}

func (a *Archive) read(ctx context.Context, name string) ([]byte, error) {
	f, ok := a.entries[name]
	if !ok {
		return nil, fmt.Errorf(locales.ModpackIsMissing(), name)
	}
	if f.UncompressedSize64 > 16<<20 {
		return nil, fmt.Errorf(locales.MetadataTooLarge(), name)
	}
	return fsutil.ReadZipFile(ctx, f, 16<<20)
}

func (a *Archive) readJSON(ctx context.Context, name string, v any) error {
	raw, err := a.read(ctx, name)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf(locales.Reading(), name, err)
	}
	return nil
}

func (a *Archive) dependency(id, version string) {
	if id == "minecraft" || id == "game" || id == "net.minecraft" {
		a.Info.GameVersion = version
		return
	}
	id = mapLoader(id)
	for _, l := range a.Info.Loaders {
		if l.ID == id && l.Version == version {
			return
		}
	}
	a.Info.Loaders = append(a.Info.Loaders, Loader{id, version})
}

func mapLoader(id string) string {
	switch id {
	case "fabric-loader", "net.fabricmc.fabric-loader":
		return "fabric"
	case "quilt-loader", "org.quiltmc.quilt-loader":
		return "quilt"
	case "net.minecraftforge":
		return "forge"
	case "neo-forge", "net.neoforged":
		return "neoforge"
	}
	return id
}

func validContentPath(name string) error {
	if _, err := fsutil.SafePath(".", name); err != nil {
		return err
	}
	parts := strings.Split(strings.ReplaceAll(name, "\\", "/"), "/")
	for _, p := range parts {
		if p == ".." || p == "." || p == "" || fsutil.ValidateID(p) != nil {
			return fmt.Errorf(locales.InvalidModpackPath(), name)
		}
	}
	if slices.Contains([]string{"versions", "libraries", "assets", "launcher_profiles.json", "launcher_accounts.json", "tomori-instance.json"}, strings.ToLower(parts[0])) {
		return fmt.Errorf(locales.ModpackContentCannotOverwriteRuntimeFiles(), name)
	}
	return nil
}
