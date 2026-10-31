package fsutil

import (
	"archive/zip"
	"context"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

func FindZipFile(reader *zip.Reader, name string) (*zip.File, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	var found *zip.File
	for _, f := range reader.File {
		if path.Clean(strings.ReplaceAll(f.Name, "\\", "/")) != name {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf(locales.DuplicateArchivePath(), name)
		}
		if !f.Mode().IsRegular() {
			return nil, fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), name)
		}
		found = f
	}
	if found == nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return found, nil
}

func ReadZipFile(ctx context.Context, file *zip.File, limit int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 0 || limit == 1<<63-1 || file.UncompressedSize64 > uint64(limit) || !file.Mode().IsRegular() {
		return nil, fmt.Errorf(locales.InvalidArchiveFileSizeOrType(), file.Name)
	}
	r, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return ReadExact(ctx, r, int64(file.UncompressedSize64))
}
