package fsutil

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

type ArchiveWriter struct {
	root               *os.Root
	maxFile, remaining uint64
	files              int
	buffer             []byte
}

func NewArchiveWriter(dir string, files int, maxFile, maxTotal uint64) (*ArchiveWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &ArchiveWriter{root: root, maxFile: maxFile, remaining: maxTotal, files: files, buffer: make([]byte, 64<<10)}, nil
}

func (w *ArchiveWriter) Close() error { return w.root.Close() }

func NewRootArchiveWriter(root *os.Root, files int, maxFile, maxTotal uint64) (*ArchiveWriter, error) {
	held, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	return &ArchiveWriter{root: held, maxFile: maxFile, remaining: maxTotal, files: files, buffer: make([]byte, 64<<10)}, nil
}

func (w *ArchiveWriter) WriteZip(ctx context.Context, f *zip.File, name string) error {
	return w.writeZip(ctx, f, name, nil, "")
}

func (w *ArchiveWriter) WriteZipSHA1(ctx context.Context, f *zip.File, name, expected string) (string, error) {
	h := sha1.New()
	err := w.writeZip(ctx, f, name, h, expected)
	return hex.EncodeToString(h.Sum(nil)), err
}

func (w *ArchiveWriter) writeZip(ctx context.Context, f *zip.File, name string, digest hash.Hash, expected string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if _, err := SafePath(".", name); err != nil {
		return err
	}
	for part := range strings.SplitSeq(name, "/") {
		if err := ValidateID(part); err != nil {
			return err
		}
	}
	if !f.Mode().IsRegular() {
		return fmt.Errorf(locales.ArchiveLinksAndSpecialFilesAreNotSupported(), f.Name)
	}
	if w.files <= 0 || f.UncompressedSize64 > w.maxFile || f.UncompressedSize64 > w.remaining {
		return errors.New(locales.ArchiveExtractionSizeOrFileCountExceedsLimit())
	}
	w.files--
	w.remaining -= f.UncompressedSize64
	dir := filepath.FromSlash(path.Dir(name))
	if err := w.root.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	temp := filepath.Join(dir, ".tomori-extract-"+rand.Text())
	out, err := w.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = w.root.Remove(temp) }()
	var sink io.Writer = out
	if digest != nil {
		sink = io.MultiWriter(out, digest)
	}
	n, err := io.CopyBuffer(sink, io.LimitReader(contextReader{ctx, in}, int64(f.UncompressedSize64)+1), w.buffer)
	if err == nil && uint64(n) != f.UncompressedSize64 {
		err = fmt.Errorf(locales.ArchiveEntrySizeMismatch(), f.Name)
	}
	if err == nil && expected != "" && !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), expected) {
		err = fmt.Errorf(locales.VerificationFailed(), name)
	}
	if err == nil {
		err = out.Chmod(0o644)
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return w.root.Rename(temp, filepath.FromSlash(name))
}
