package launcher

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) copyLegacyFiles(ctx context.Context, copier *fsutil.TreeCopier, root, destination *os.Root, instance minecraft.Instance) error {
	detail, err := minecraft.ReadLocalVersion(instance.GameDir, instance.ID)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	file := func(path string, required bool) error {
		path = filepath.FromSlash(path)
		if seen[path] {
			return nil
		}
		seen[path] = true
		return copier.File(root, path, path, required)
	}
	client := detail.ClientID()
	if _, err := root.Lstat(filepath.Join("versions", client, client+".jar")); errors.Is(err, os.ErrNotExist) && detail.Jar == "" {
		parent, err := copyClientParent(root, instance.ID, detail)
		if err != nil {
			return err
		}
		if parent != "" {
			client, detail.Jar = parent, parent
		}
	}
	if err := file(filepath.Join("versions", client, client+".jar"), false); err != nil {
		return err
	}
	env := minecraft.CurrentEnvironment()
	for _, library := range detail.Libraries {
		if err := ctx.Err(); err != nil {
			return err
		}
		ref, err := library.Artifact()
		if err != nil {
			return err
		}
		applies := library.Applies(env)
		if ref != nil {
			if err := file(filepath.Join("libraries", ref.Path), applies && ref.URL == ""); err != nil {
				return err
			}
		}
		native := minecraft.NativeClassifier(library.Natives, env)
		for classifier, ref := range library.Downloads.Classifiers {
			if err := file(filepath.Join("libraries", ref.Path), applies && classifier == native && ref.URL == ""); err != nil {
				return err
			}
		}
	}
	if detail.Logging != nil && detail.Logging.Client != nil {
		name := detail.Logging.Client.File.ID()
		if err := fsutil.ValidateID(name); err != nil {
			return err
		}
		if err := file(filepath.Join("assets", "log_configs", name), false); err != nil {
			return err
		}
	}
	if detail.AssetIndex.ID != "" {
		if err := fsutil.ValidateID(detail.AssetIndex.ID); err != nil {
			return err
		}
		path := filepath.Join("assets", "indexes", detail.AssetIndex.ID+".json")
		if err := file(path, false); err != nil {
			return err
		}
		if err := copyAssetObjects(ctx, root, path, file); err != nil {
			return err
		}
	}
	if err := copier.Close(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return fsutil.WriteRootAtomic(ctx, destination, filepath.Join("versions", detail.ID, detail.ID+".json"), raw)
}

func copyClientParent(root *os.Root, id string, detail *minecraft.VersionDetail) (string, error) {
	for range 32 {
		if err := fsutil.ValidateID(id); err != nil {
			return "", err
		}
		raw, err := fsutil.ReadRootFile(root, filepath.Join("versions", id, id+".json"), 16<<20)
		if err != nil {
			return "", err
		}
		var source minecraft.VersionDetail
		if err := json.Unmarshal(raw, &source); err != nil {
			return "", err
		}
		if source.ID == "" {
			source.ID = id
		}
		ref := source.Downloads.Client
		matches := ref.URL != "" && ref.URL == detail.Downloads.Client.URL
		if detail.Downloads.Client.SHA1 != "" {
			matches = ref.SHA1 == detail.Downloads.Client.SHA1
		}
		client := source.ClientID()
		if err := fsutil.ValidateID(client); err != nil {
			return "", err
		}
		if matches {
			if _, err := root.Lstat(filepath.Join("versions", client, client+".jar")); err == nil {
				return client, nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return "", err
			}
		}
		if source.InheritsFrom == "" {
			return "", nil
		}
		id = source.InheritsFrom
	}
	return "", errors.New(locales.InstanceDirectoryNestingIsTooDeep())
}

func copyAssetObjects(ctx context.Context, root *os.Root, path string, copy func(string, bool) error) error {
	f, err := fsutil.OpenRootRegular(root, path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() > 16<<20 {
		return errors.New(locales.InvalidInstanceFileTypeOrSize())
	}
	decoder := json.NewDecoder(io.LimitReader(f, 16<<20+1))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token != json.Delim('{') {
		return errors.New(locales.InvalidAssetIndex())
	}
	count := 0
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return err
		}
		if key != "objects" {
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return err
			}
			continue
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token != json.Delim('{') {
			return errors.New(locales.InvalidAssetIndex())
		}
		for decoder.More() {
			if err := ctx.Err(); err != nil {
				return err
			}
			count++
			if count > 200000 {
				return errors.New(locales.InstanceFileCountOrSizeExceedsTheCopyLimit())
			}
			if _, err := decoder.Token(); err != nil {
				return err
			}
			var object struct {
				Hash string
				Size int64
			}
			if err := decoder.Decode(&object); err != nil {
				return err
			}
			decoded, err := hex.DecodeString(object.Hash)
			if err != nil || len(decoded) != 20 || object.Size < 0 || object.Size > 64<<30 || object.Hash != strings.ToLower(object.Hash) {
				return errors.New(locales.InvalidAssetIndex())
			}
			if err := copy(filepath.Join("assets", "objects", object.Hash[:2], object.Hash), false); err != nil {
				return err
			}
		}
		if _, err := decoder.Token(); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New(locales.InvalidAssetIndex())
	}
	return nil
}
