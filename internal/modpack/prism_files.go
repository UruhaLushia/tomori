package modpack

import (
	"context"
	"path/filepath"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

func (a *Archive) ExtractPrismLibraries(ctx context.Context, destination string) error {
	for relative, file := range a.PrismLibraries {
		path, err := fsutil.SafePath(destination, relative)
		if err != nil {
			return err
		}
		raw, err := fsutil.ReadZipFile(ctx, file, 4<<30)
		if err != nil {
			return err
		}
		if err := fsutil.WriteAtomic(filepath.Clean(path), raw); err != nil {
			return err
		}
	}
	return nil
}
