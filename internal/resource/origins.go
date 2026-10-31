package resource

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

const originLimit = 4 << 20

type Origin struct {
	Source    string `json:"source"`
	ProjectID string `json:"project"`
	VersionID string `json:"version"`
	Number    string `json:"number,omitempty"`
}

type OriginFile struct {
	Kind   Kind   `json:"kind"`
	SHA1   string `json:"sha1"`
	Size   int64  `json:"size"`
	Origin Origin `json:"origin"`
}

type Origins struct {
	Version int          `json:"version"`
	Files   []OriginFile `json:"files"`
	changed bool
}

func ScanWithOrigins(ctx context.Context, directory, gameDir string, kind Kind) ([]Entry, error) {
	entries, err := Scan(ctx, gameDir, kind)
	if err != nil {
		return nil, err
	}
	origins, err := LoadOrigins(directory, gameDir)
	if err == nil {
		err = origins.Attach(ctx, gameDir, kind, entries)
	}
	return entries, err
}

func originName(gameDir string) (string, error) {
	path, err := filepath.EvalSymlinks(gameDir)
	if err != nil {
		return "", err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:]) + ".json", nil
}

func originRoot(directory string, create bool) (*os.Root, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err := root.Mkdir("resource-origins", 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	return root.OpenRoot("resource-origins")
}

func LoadOrigins(directory, gameDir string) (*Origins, error) {
	index := &Origins{Version: 1}
	name, err := originName(gameDir)
	if err != nil {
		return nil, err
	}
	root, err := originRoot(directory, false)
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > originLimit {
		return nil, errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	raw, err := io.ReadAll(io.LimitReader(f, originLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > originLimit {
		return nil, errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	*index = Origins{}
	if err := json.Unmarshal(raw, index); err != nil {
		return nil, err
	}
	if err := index.validate(); err != nil {
		return nil, err
	}
	return index, nil
}

func (o *Origins) validate() error {
	if o.Version != 1 || len(o.Files) > 10000 {
		return errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	seen := make(map[string]bool, len(o.Files))
	for _, file := range o.Files {
		if err := file.validate(); err != nil {
			return err
		}
		key := string(file.Kind) + ":" + file.SHA1 + ":" + file.Origin.Source
		if seen[key] {
			return errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
		}
		seen[key] = true
	}
	return nil
}

func (f OriginFile) validate() error {
	sum, err := hex.DecodeString(f.SHA1)
	validID := func(id string) bool {
		if len(id) == 0 || len(id) > 128 {
			return false
		}
		for _, b := range []byte(id) {
			switch {
			case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '-', b == '_':
			default:
				return false
			}
		}
		return true
	}
	if !f.Kind.Valid() || err != nil || len(sum) != 20 || f.SHA1 != strings.ToLower(f.SHA1) || f.Size <= 0 ||
		(f.Origin.Source != "modrinth" && f.Origin.Source != "curseforge") || !validID(f.Origin.ProjectID) || !validID(f.Origin.VersionID) || len(f.Origin.Number) > 256 {
		return errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	return nil
}

func (o *Origins) Remember(file OriginFile) error {
	file.SHA1 = strings.ToLower(file.SHA1)
	if err := file.validate(); err != nil {
		return err
	}
	for i, existing := range o.Files {
		if existing.Kind == file.Kind && existing.SHA1 == file.SHA1 && existing.Origin.Source == file.Origin.Source {
			if existing != file {
				o.Files[i], o.changed = file, true
			}
			return nil
		}
	}
	if len(o.Files) >= 10000 {
		return errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	o.Files = append(o.Files, file)
	o.changed = true
	return nil
}

func (o *Origins) Changed() bool { return o.changed }

func (o *Origins) Retain(keep func(OriginFile) bool) {
	retained := o.Files[:0]
	for _, file := range o.Files {
		if keep(file) {
			retained = append(retained, file)
		}
	}
	if len(retained) != len(o.Files) {
		clear(o.Files[len(retained):])
		o.Files, o.changed = retained, true
	}
}

func (o *Origins) Save(directory, gameDir string) error {
	if err := o.validate(); err != nil {
		return err
	}
	name, err := originName(gameDir)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(o)
	if err != nil {
		return err
	}
	if len(raw) > originLimit {
		return errors.New(locales.ResourceOriginIndexIsInvalidOrTooLarge())
	}
	root, err := originRoot(directory, true)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := fsutil.WriteRootAtomic(context.Background(), root, name, raw); err != nil {
		return err
	}
	o.changed = false
	return nil
}

func (o *Origins) Attach(ctx context.Context, gameDir string, kind Kind, entries []Entry) error {
	bySize := make(map[int64]bool)
	byHash := make(map[string][]Origin)
	for _, file := range o.Files {
		if file.Kind == kind {
			bySize[file.Size] = true
			byHash[file.SHA1] = append(byHash[file.SHA1], file.Origin)
		}
	}
	for i := range entries {
		entry := &entries[i]
		entry.Origins = nil
		if entry.Directory || !bySize[entry.Size] {
			continue
		}
		hash, err := HashEntry(ctx, gameDir, kind, *entry)
		if err != nil {
			return err
		}
		entry.Origins = byHash[hash]
	}
	return ctx.Err()
}
