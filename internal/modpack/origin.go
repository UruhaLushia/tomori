package modpack

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

const OriginFile = ".tomori-pack-origin.json"
const originLimit = 128 << 20

type ArchiveIdentity struct {
	Name   string `json:"name"`
	SHA512 string `json:"sha512"`
	Size   int64  `json:"size"`
}

type InstalledFile struct {
	Path      string `json:"path"`
	SHA1      string `json:"sha1"`
	Size      int64  `json:"size"`
	ProjectID int64  `json:"project,omitempty"`
	FileID    int64  `json:"file,omitempty"`
	Optional  bool   `json:"optional,omitempty"`
	Override  bool   `json:"override,omitempty"`
}

type PackMetadata struct {
	Format      string   `json:"format"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Author      string   `json:"author,omitempty"`
	GameVersion string   `json:"gameVersion"`
	Loaders     []Loader `json:"loaders,omitempty"`
}

type OriginHeader struct {
	Schema      int                  `json:"schema"`
	Source      minecraft.PackSource `json:"source"`
	Archive     ArchiveIdentity      `json:"archive"`
	Pack        PackMetadata         `json:"pack"`
	Optional    bool                 `json:"optional"`
	InstalledAt time.Time            `json:"installedAt"`
	FileCount   int                  `json:"fileCount"`
}

type Origin struct {
	Header    OriginHeader
	files     map[string]InstalledFile
	pathBytes int
	fileBytes int64
}

func NewOrigin(info Info, source minecraft.PackSource, archive ArchiveIdentity, optional bool) (*Origin, error) {
	header := OriginHeader{Schema: 1, Source: source, Archive: archive, Optional: optional, InstalledAt: time.Now().UTC(), Pack: PackMetadata{Format: info.Format, Name: info.Name, Version: info.Version, Author: info.Author, GameVersion: info.GameVersion, Loaders: slices.Clone(info.Loaders)}}
	if err := header.validate(); err != nil {
		return nil, err
	}
	return &Origin{Header: header, files: make(map[string]InstalledFile)}, nil
}

func invalidOrigin() error { return errors.New(locales.InvalidOrOversizedModpackOriginRecord()) }

func ValidateSource(source minecraft.PackSource) error {
	if source.Provider == "" {
		if source.ProjectID == "" && source.VersionID == "" && source.Number == "" {
			return nil
		}
		return invalidOrigin()
	}
	if source.Provider != "modrinth" && source.Provider != "curseforge" || len(source.Number) > 2048 || !utf8.ValidString(source.Number) || strings.ContainsFunc(source.Number, unicode.IsControl) {
		return invalidOrigin()
	}
	for _, id := range []string{source.ProjectID, source.VersionID} {
		if id == "" || len(id) > 128 {
			return invalidOrigin()
		}
		if source.Provider == "curseforge" {
			n, err := strconv.ParseInt(id, 10, 64)
			if err != nil || n <= 0 || strconv.FormatInt(n, 10) != id {
				return invalidOrigin()
			}
		} else {
			for _, r := range id {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
					return invalidOrigin()
				}
			}
		}
	}
	return nil
}

func (h OriginHeader) validate() error {
	if h.Schema != 1 || h.FileCount < 0 || h.FileCount > 200000 || h.Archive.Size <= 0 || !validHash(h.Archive.SHA512, 64) || fsutil.ValidateID(h.Archive.Name) != nil || h.InstalledAt.IsZero() || len(h.Pack.Loaders) > 8 {
		return invalidOrigin()
	}
	if err := ValidateSource(h.Source); err != nil {
		return err
	}
	if err := fsutil.ValidateID(h.Pack.GameVersion); err != nil {
		return invalidOrigin()
	}
	for _, loader := range h.Pack.Loaders {
		if !slices.Contains([]string{"fabric", "quilt", "forge", "neoforge"}, loader.ID) || fsutil.ValidateID(loader.Version) != nil {
			return invalidOrigin()
		}
	}
	for _, value := range []string{h.Pack.Format, h.Pack.Name, h.Pack.Version, h.Pack.Author} {
		if !utf8.ValidString(value) || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return invalidOrigin()
		}
	}
	return nil
}

func (o *Origin) Put(file InstalledFile) error {
	file.Path = strings.ReplaceAll(file.Path, "\\", "/")
	if len(file.Path) > 4096 {
		return invalidOrigin()
	}
	if err := validContentPath(file.Path); err != nil {
		return err
	}
	if !validHash(file.SHA1, 20) || file.Size < 0 || file.Size > 4<<30 || file.ProjectID < 0 || file.FileID < 0 || (file.ProjectID == 0) != (file.FileID == 0) || file.Override && (file.ProjectID != 0 || file.Optional) {
		return invalidOrigin()
	}
	file.SHA1 = strings.ToLower(file.SHA1)
	key := strings.ToLower(file.Path)
	old, found := o.files[key]
	if found && old.Path != file.Path {
		return invalidOrigin()
	}
	paths := o.pathBytes + len(file.Path) - len(old.Path)
	bytes := o.fileBytes + file.Size - old.Size
	if !found && len(o.files) >= 200000 || paths > 16<<20 || bytes > 64<<30 {
		return invalidOrigin()
	}
	o.files[key] = file
	o.pathBytes, o.fileBytes = paths, bytes
	o.Header.FileCount = len(o.files)
	return nil
}

func (o *Origin) Files() []InstalledFile {
	files := make([]InstalledFile, 0, len(o.files))
	for _, file := range o.files {
		files = append(files, file)
	}
	slices.SortFunc(files, func(a, b InstalledFile) int { return strings.Compare(a.Path, b.Path) })
	return files
}

func (o *Origin) EstimatedSize() uint64 {
	return uint64(min(originLimit, (64<<10)+o.pathBytes*6+len(o.files)*320))
}

func (o *Origin) Write(ctx context.Context, root *os.Root) error {
	current := o.Header
	current.FileCount = len(o.files)
	if err := current.validate(); err != nil {
		return err
	}
	header, err := json.Marshal(current)
	if err != nil || len(header) > 60<<10 {
		return invalidOrigin()
	}
	return fsutil.WriteRootAtomicStream(ctx, root, OriginFile, func(out io.Writer) error {
		w := bufio.NewWriterSize(&originWriter{out: out}, 64<<10)
		if _, err := w.WriteString("{\"header\":" + string(header) + ",\"files\":[\n"); err != nil {
			return err
		}
		encoder := json.NewEncoder(w)
		first := true
		for _, file := range o.files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !first {
				if err := w.WriteByte(','); err != nil {
					return err
				}
			}
			first = false
			if err := encoder.Encode(file); err != nil {
				return err
			}
		}
		if _, err := w.WriteString("]}\n"); err != nil {
			return err
		}
		return w.Flush()
	})
}

func ReadOriginHeader(ctx context.Context, root *os.Root) (OriginHeader, error) {
	if err := ctx.Err(); err != nil {
		return OriginHeader{}, err
	}
	f, err := fsutil.OpenRootRegular(root, OriginFile)
	if err != nil {
		return OriginHeader{}, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(fsutil.ContextReader(ctx, f), 64<<10)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return OriginHeader{}, originReadError(ctx, err)
	}
	return readOriginHeader(json.NewDecoder(bytes.NewReader(line)))
}

func readOriginHeader(decoder *json.Decoder) (OriginHeader, error) {
	var header OriginHeader
	decoder.DisallowUnknownFields()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return header, invalidOrigin()
	}
	if token, err := decoder.Token(); err != nil || token != "header" {
		return header, invalidOrigin()
	}
	if err := decoder.Decode(&header); err != nil {
		return header, invalidOrigin()
	}
	if err := header.validate(); err != nil {
		return header, err
	}
	if token, err := decoder.Token(); err != nil || token != "files" {
		return header, invalidOrigin()
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('[') {
		return header, invalidOrigin()
	}
	if _, err := decoder.Token(); err != io.EOF {
		return header, invalidOrigin()
	}
	return header, nil
}

func ReadOrigin(ctx context.Context, root *os.Root) (*Origin, error) {
	return readOrigin(ctx, root, nil)
}

func ReadOriginSnapshot(ctx context.Context, root *os.Root) (*Origin, string, error) {
	hash := sha1.New()
	origin, err := readOrigin(ctx, root, hash)
	if err != nil {
		return nil, "", err
	}
	return origin, hex.EncodeToString(hash.Sum(nil)), nil
}

func readOrigin(ctx context.Context, root *os.Root, digest io.Writer) (*Origin, error) {
	f, err := fsutil.OpenRootRegular(root, OriginFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if before.Size() > originLimit {
		return nil, invalidOrigin()
	}
	var in io.Reader = f
	if digest != nil {
		in = io.TeeReader(in, digest)
	}
	r := bufio.NewReaderSize(io.LimitReader(fsutil.ContextReader(ctx, in), originLimit+1), 64<<10)
	line, err := r.ReadSlice('\n')
	if err != nil {
		return nil, originReadError(ctx, err)
	}
	header, err := readOriginHeader(json.NewDecoder(bytes.NewReader(line)))
	if err != nil {
		return nil, err
	}
	o := &Origin{Header: header, files: make(map[string]InstalledFile)}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err = r.ReadSlice('\n')
		if err != nil {
			return nil, originReadError(ctx, err)
		}
		line = bytes.TrimSpace(line)
		if bytes.Equal(line, []byte("]}")) {
			break
		}
		if len(o.files) > 0 {
			if len(line) == 0 || line[0] != ',' {
				return nil, invalidOrigin()
			}
			line = line[1:]
		}
		var file InstalledFile
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&file); err != nil {
			return nil, originReadError(ctx, err)
		}
		if _, err := decoder.Token(); err != io.EOF {
			return nil, invalidOrigin()
		}
		if _, found := o.files[strings.ToLower(strings.ReplaceAll(file.Path, "\\", "/"))]; found {
			return nil, invalidOrigin()
		}
		if err := o.Put(file); err != nil {
			return nil, err
		}
	}
	if len(o.files) != header.FileCount {
		return nil, invalidOrigin()
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return nil, invalidOrigin()
	}
	after, err := f.Stat()
	if err != nil {
		return nil, err
	}
	latest, err := root.Lstat(OriginFile)
	if err != nil {
		return nil, err
	}
	if !sameUpdateFile(before, after) || !sameUpdateFile(before, latest) {
		return nil, invalidOrigin()
	}
	return o, ctx.Err()
}

func originReadError(ctx context.Context, _ error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return invalidOrigin()
}

type originWriter struct {
	out io.Writer
	n   int
}

func (w *originWriter) Write(p []byte) (int, error) {
	if len(p) > originLimit-w.n {
		return 0, invalidOrigin()
	}
	n, err := w.out.Write(p)
	w.n += n
	return n, err
}
