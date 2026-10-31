package instancecontent

import (
	"compress/gzip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func WorldName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 128 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", errors.New(locales.Use1128CharactersWithoutControlCharacters())
	}
	return name, nil
}

func RenameWorld(ctx context.Context, root *os.Root, name string) error {
	name, err := WorldName(name)
	if err != nil {
		return err
	}
	f, err := fsutil.OpenRootRegular(root, "level.dat")
	if err != nil {
		return err
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		return err
	}
	n, err := parseWorld(ctx, f, false)
	if err != nil {
		return err
	}
	if n.duplicate || n.dataEnd < 0 {
		return errors.New(locales.CouldNotReadWorldDetails())
	}
	encoded := encodeWorldText(name)
	start, end := n.nameStart, n.nameEnd
	if start < 0 {
		start, end = n.dataEnd, n.dataEnd
		encoded = append([]byte{8, 0, 9, 'L', 'e', 'v', 'e', 'l', 'N', 'a', 'm', 'e'}, encoded...)
	}
	return rewriteWorld(ctx, root, f, before, n, []worldEdit{{start, end, encoded}})
}

type worldEdit struct {
	start, end int64
	data       []byte
}

func rewriteWorld(ctx context.Context, root *os.Root, f *os.File, before os.FileInfo, n worldNBT, edits []worldEdit) error {
	slices.SortStableFunc(edits, func(a, b worldEdit) int {
		if a.start < b.start {
			return -1
		}
		if a.start > b.start {
			return 1
		}
		return 0
	})
	size, previous := n.offset(), int64(0)
	for _, edit := range edits {
		if edit.start < previous || edit.end < edit.start || edit.end > n.offset() {
			return errors.New(locales.CouldNotReadWorldDetails())
		}
		previous = edit.end
		size += int64(len(edit.data)) - (edit.end - edit.start)
	}
	if size > 8<<20 {
		return errors.New(locales.WorldMetadataExceedsTheSizeLimit())
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	z, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer z.Close()
	return fsutil.WriteRootAtomicStream(ctx, root, "level.dat", func(out io.Writer) error {
		writer := gzip.NewWriter(&worldWriter{Writer: out, remaining: 8 << 20})
		defer writer.Close()
		in := &worldReader{ctx: ctx, Reader: io.LimitReader(z, (8<<20)+1)}
		for _, edit := range edits {
			if _, err := io.CopyN(writer, in, edit.start-in.read); err != nil {
				return err
			}
			if _, err := io.CopyN(io.Discard, in, edit.end-edit.start); err != nil {
				return err
			}
			if _, err := writer.Write(edit.data); err != nil {
				return err
			}
		}
		if _, err := io.Copy(writer, in); err != nil {
			return err
		}
		if in.read != n.offset() {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		if err := writer.Close(); err != nil {
			return err
		}
		after, err := root.Lstat("level.dat")
		if err != nil {
			return err
		}
		if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return f.Close()
	})
}

type worldWriter struct {
	io.Writer
	remaining int
}

func (w *worldWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, errors.New(locales.WorldMetadataExceedsTheSizeLimit())
	}
	n, err := w.Writer.Write(p)
	w.remaining -= n
	return n, err
}

func encodeWorldText(text string) []byte {
	out := make([]byte, 2, 2+len(text)*2)
	for _, unit := range utf16.Encode([]rune(text)) {
		switch {
		case unit > 0 && unit < 0x80:
			out = append(out, byte(unit))
		case unit < 0x800:
			out = append(out, 0xc0|byte(unit>>6), 0x80|byte(unit&0x3f))
		default:
			out = append(out, 0xe0|byte(unit>>12), 0x80|byte(unit>>6&0x3f), 0x80|byte(unit&0x3f))
		}
	}
	binary.BigEndian.PutUint16(out[:2], uint16(len(out)-2))
	return out
}
