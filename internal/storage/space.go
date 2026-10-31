package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/shirou/gopsutil/v4/disk"

	"github.com/UruhaLushia/tomori/locales"
)

type Requirement struct {
	Directory string
	Bytes     uint64
	Unknown   bool
}

type Volume struct {
	Directory, Key      string
	Required, Available uint64
	Unknown             bool
}

type Report struct{ Volumes []Volume }

type InsufficientSpace struct{ Volume Volume }

func IsFull(err error) bool {
	var space *InsufficientSpace
	return errors.As(err, &space) || diskFull(err)
}

func (e *InsufficientSpace) Error() string {
	return fmt.Sprintf(locales.NotEnoughDiskSpaceNeedsAvailable(), e.Volume.Directory, Size(e.Volume.Required), Size(e.Volume.Available))
}

func (r Report) Check() error {
	for _, v := range r.Volumes {
		if v.Required > v.Available {
			return &InsufficientSpace{Volume: v}
		}
	}
	return nil
}

func Add(a, b uint64) uint64 {
	if b > math.MaxUint64-a {
		return math.MaxUint64
	}
	return a + b
}

func Size(n uint64) string {
	for i, unit := range []string{"TiB", "GiB", "MiB", "KiB"} {
		shift := uint(40 - i*10)
		if n >= 1<<shift {
			return fmt.Sprintf("%.1f %s", float64(n)/float64(uint64(1)<<shift), unit)
		}
	}
	return fmt.Sprintf("%d B", n)
}

func Assess(ctx context.Context, requirements []Requirement) (Report, error) {
	var result Report
	parents := make(map[string]string)
	volumes := make(map[string]int)
	var existing func(string) (string, error)
	existing = func(path string) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if found, ok := parents[path]; ok {
			return found, nil
		}
		info, err := os.Stat(path)
		var found string
		switch {
		case err == nil && info.IsDir():
			found, err = filepath.EvalSymlinks(path)
		case err == nil:
			return "", fmt.Errorf(locales.NotADirectory(), path)
		case os.IsNotExist(err) && filepath.Dir(path) != path:
			found, err = existing(filepath.Dir(path))
		}
		if err != nil {
			return "", err
		}
		parents[path] = found
		return found, nil
	}
	for _, need := range requirements {
		path, err := filepath.Abs(need.Directory)
		if err != nil || need.Directory == "" {
			return result, errors.New(locales.InvalidStorageDirectory())
		}
		path, err = existing(path)
		if err != nil {
			return result, err
		}
		key, err := volumeKey(path)
		if err != nil {
			return result, err
		}
		index, found := volumes[key]
		if !found {
			usage, err := disk.UsageWithContext(ctx, path)
			if err != nil {
				return result, err
			}
			index = len(result.Volumes)
			volumes[key] = index
			result.Volumes = append(result.Volumes, Volume{Directory: path, Key: key, Available: usage.Free})
		}
		v := &result.Volumes[index]
		v.Required = Add(v.Required, need.Bytes)
		v.Unknown = v.Unknown || need.Unknown
	}
	return result, ctx.Err()
}
