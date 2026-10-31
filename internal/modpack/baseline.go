package modpack

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func (a *Archive) RecordBaseline(ctx context.Context, origin *Origin, files []File) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.ReplaceAll(file.Path, "\\", "/")
		key := strings.ToLower(name)
		if previous, found := origin.files[key]; found {
			return fmt.Errorf(locales.ModpackPathConflict(), previous.Path, name)
		}
		if err := origin.Put(InstalledFile{Path: file.Path, SHA1: file.SHA1, Size: file.Size, ProjectID: file.ProjectID, FileID: file.FileID, Optional: file.Optional}); err != nil {
			return err
		}
	}
	buffer := archiveBuffers.Get().(*[]byte)
	defer archiveBuffers.Put(buffer)
	w := baselineWriter{buffer: *buffer}
	return a.extractRecorded(ctx, &w, nil, origin.Put)
}

type baselineWriter struct {
	buffer []byte
}

func (w *baselineWriter) WriteZip(ctx context.Context, f *zip.File, name string) error {
	_, err := w.WriteZipSHA1(ctx, f, name, "")
	return err
}

func (w *baselineWriter) WriteZipSHA1(ctx context.Context, f *zip.File, name, expected string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if !f.Mode().IsRegular() || f.UncompressedSize64 > 4<<30 {
		return "", invalidOrigin()
	}
	in, err := f.Open()
	if err != nil {
		return "", err
	}
	defer in.Close()
	h := sha1.New()
	n, err := io.CopyBuffer(h, io.LimitReader(fsutil.ContextReader(ctx, in), int64(f.UncompressedSize64)+1), w.buffer)
	if err != nil {
		return "", err
	}
	if uint64(n) != f.UncompressedSize64 {
		return "", fmt.Errorf(locales.ArchiveEntrySizeMismatch(), f.Name)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if expected != "" && !strings.EqualFold(sum, expected) {
		return "", fmt.Errorf(locales.VerificationFailed(), name)
	}
	return sum, ctx.Err()
}
