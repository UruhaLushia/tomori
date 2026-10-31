package fsutil

import (
	"archive/zip"
	"compress/flate"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type ZipExporter struct {
	ctx       context.Context
	zip       *zip.Writer
	permit    func(context.Context) (func(), error)
	progress  func(CopyProgress)
	buffer    []byte
	paths     map[string]string
	pathBytes int
	state     CopyProgress
	last      time.Time
}

func NewZipExporter(ctx context.Context, out io.Writer, permit func(context.Context) (func(), error), progress func(CopyProgress)) *ZipExporter {
	z := zip.NewWriter(out)
	var compressor *flate.Writer
	z.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		if compressor == nil {
			var err error
			compressor, err = flate.NewWriter(w, flate.BestSpeed)
			return compressor, err
		}
		compressor.Reset(w)
		return compressor, nil
	})
	return &ZipExporter{ctx: ctx, zip: z, permit: permit, progress: progress, buffer: make([]byte, 64<<10), paths: make(map[string]string)}
}

func (z *ZipExporter) Close() error { return z.zip.Close() }

func (z *ZipExporter) Progress() CopyProgress { return z.state }

func (z *ZipExporter) header(name string, size int64, modified time.Time) (io.Writer, error) {
	if err := z.ctx.Err(); err != nil {
		return nil, err
	}
	if err := copyPath(filepath.FromSlash(name)); err != nil {
		return nil, err
	}
	key := strings.ToLower(name)
	if old, found := z.paths[key]; found {
		return nil, fmt.Errorf(locales.ModpackPathConflict(), old, name)
	}
	if len(z.paths) >= 200000 || len(name) > 16<<20-z.pathBytes || size < 0 || size > 4<<30 || size > 64<<30-z.state.TotalBytes {
		return nil, errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
	}
	z.paths[key] = name
	z.pathBytes += len(name)
	z.state.TotalBytes += size
	z.state.TotalFiles++
	header := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: modified}
	header.SetMode(0644)
	switch strings.ToLower(filepath.Ext(strings.TrimSuffix(name, ".disabled"))) {
	case ".jar", ".zip", ".png", ".jpg", ".jpeg", ".webp", ".ogg", ".gz", ".xz", ".7z", ".mrpack":
		header.Method = zip.Store
	}
	return z.zip.CreateHeader(header)
}

func (z *ZipExporter) Bytes(name string, raw []byte) error {
	out, err := z.header(name, int64(len(raw)), time.Time{})
	if err != nil {
		return err
	}
	n, err := out.Write(raw)
	z.state.Bytes += int64(n)
	if err == nil {
		z.state.Files++
		z.report()
	}
	return err
}

func (z *ZipExporter) File(root *os.Root, source, name string, expected os.FileInfo) error {
	return z.file(root, source, name, expected, nil)
}

func (z *ZipExporter) FileSHA1(root *os.Root, source, name string, expected os.FileInfo) (string, error) {
	hash := sha1.New()
	if err := z.file(root, source, name, expected, hash); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func (z *ZipExporter) file(root *os.Root, source, name string, expected os.FileInfo, digest io.Writer) error {
	if err := z.ctx.Err(); err != nil {
		return err
	}
	if z.permit != nil {
		release, err := z.permit(z.ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	in, err := OpenRootRegular(root, source)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if expected != nil && !sameCopyFile(expected, before) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	out, err := z.header(name, before.Size(), before.ModTime())
	if err != nil {
		return err
	}
	if digest != nil {
		out = io.MultiWriter(out, digest)
	}
	n, err := io.CopyBuffer(zipOutput{out, z}, io.LimitReader(contextReader{z.ctx, in}, before.Size()+1), z.buffer)
	if err != nil {
		return err
	}
	after, err := in.Stat()
	if err != nil {
		return err
	}
	current, err := root.Lstat(source)
	if err != nil {
		return err
	}
	if n != before.Size() || !sameCopyFile(before, after) || !sameCopyFile(before, current) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	z.state.Files++
	z.report()
	return z.ctx.Err()
}

func (z *ZipExporter) Tree(root *os.Root, prefix string, include func(string, bool) bool, reference func(string, os.FileInfo) (bool, error)) error {
	return WalkFiles(z.ctx, root, include, func(name string, info os.FileInfo) error {
		if reference != nil {
			skip, err := reference(filepath.ToSlash(name), info)
			if err != nil || skip {
				return err
			}
		}
		return z.File(root, name, prefix+filepath.ToSlash(name), info)
	})
}

type zipOutput struct {
	io.Writer
	zip *ZipExporter
}

func (w zipOutput) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.zip.state.Bytes += int64(n)
	w.zip.report()
	return n, err
}

func (z *ZipExporter) report() {
	if z.progress != nil && time.Since(z.last) >= 100*time.Millisecond {
		z.last = time.Now()
		z.progress(z.state)
	}
}
