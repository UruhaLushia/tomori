package launcher

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/internal/resourcecatalog"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) SupplementResources(ctx context.Context, files []resourcecatalog.MissingFile, paths []string) (int, error) {
	if len(files) == 0 || len(files) > 1000 || len(paths) == 0 || len(paths) > 1000 {
		return 0, errors.New(locales.InvalidResourceUpdateCountOrLimitExceeded())
	}
	byHash := make(map[string][]resourcecatalog.MissingFile, len(files))
	sizes := make(map[int64]bool, len(files))
	for _, file := range files {
		hash := strings.ToLower(file.SHA1)
		decoded, err := hex.DecodeString(hash)
		if err != nil || len(decoded) != 20 || file.Size <= 0 || !file.Kind.Valid() || !file.Kind.Accepts(file.Name, false) {
			return 0, errors.New(locales.InvalidResourceChecksum())
		}
		if err := fsutil.ValidateID(file.Name); err != nil {
			return 0, err
		}
		file.SHA1 = hash
		byHash[hash] = append(byHash[hash], file)
		sizes[file.Size] = true
	}
	var problems []error
	count := 0
	seen := make(map[string]bool, len(paths))
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || !sizes[info.Size()] {
			problems = append(problems, fmt.Errorf(locales.FileDoesNotMatch(), filepath.Base(path)))
			continue
		}
		release, err := s.client.FilePermit(ctx)
		if err != nil {
			return count, err
		}
		hash, err := resource.Hash(ctx, path)
		release()
		if err != nil {
			problems = append(problems, err)
			continue
		}
		matches := byHash[hash]
		if len(matches) == 0 {
			problems = append(problems, fmt.Errorf(locales.FileDoesNotMatch(), filepath.Base(path)))
			continue
		}
		for _, file := range matches {
			if file.Size != info.Size() {
				problems = append(problems, fmt.Errorf(locales.FileDoesNotMatch(), filepath.Base(path)))
				continue
			}
			key := hash + ":" + file.Name
			if seen[key] {
				continue
			}
			path := cache.File(s.cacheDir, hash)
			task := download.Task{Path: path, SHA1: hash, Size: file.Size}
			if err := s.client.ReuseLocal(ctx, task, func(download.Task) []string { return []string{path} }); err != nil {
				problems = append(problems, err)
				continue
			}
			seen[key] = true
			count++
		}
	}
	if err := ctx.Err(); err != nil {
		return count, err
	}
	return count, errors.Join(problems...)
}
