package fsutil

import (
	"archive/zip"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"

	"github.com/UruhaLushia/tomori/locales"
)

const (
	maxZipFiles     = 200000
	maxZipDirectory = 32 << 20
	zipDirectorySig = 0x02014b50
	zipEndSig       = 0x06054b50
	zip64EndSig     = 0x06064b50
	zip64LocatorSig = 0x07064b50
)

type ZipReadCloser struct {
	*zip.Reader
	file *os.File
}

func (r *ZipReadCloser) Close() error { return r.file.Close() }

func OpenZip(ctx context.Context, name string) (*ZipReadCloser, error) {
	f, err := OpenRegular(name)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	var reader *zip.Reader
	if err == nil {
		reader, err = NewZipReader(ctx, f, info.Size())
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &ZipReadCloser{Reader: reader, file: f}, nil
}

func NewZipReader(ctx context.Context, source io.ReaderAt, size int64) (*zip.Reader, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if size < 22 {
		return nil, zip.ErrFormat
	}
	r := &zipSnapshot{source: source, ctx: ctx}
	defer func() { r.regions, r.ctx = nil, nil }()
	d, err := readZipDirectory(r, size)
	if err != nil {
		return nil, err
	}
	if d.records > maxZipFiles || d.size > maxZipDirectory {
		return nil, errors.New(locales.ArchiveDirectorySizeOrEntryCountExceedsTheLimit())
	}
	if d.size > uint64(d.end) || d.offset > uint64(d.end)-d.size {
		return nil, zip.ErrFormat
	}
	start := d.end - int64(d.size)
	if start > int64(d.offset) {
		var header [46]byte
		if _, err := source.ReadAt(header[:], int64(d.offset)); err == nil && binary.LittleEndian.Uint32(header[:]) == zipDirectorySig {
			start = int64(d.offset)
		}
	}
	if d.end-start > maxZipDirectory {
		return nil, errors.New(locales.ArchiveDirectorySizeOrEntryCountExceedsTheLimit())
	}
	data, err := r.capture(start, int(d.end-start))
	if err != nil {
		return nil, err
	}
	count, err := countZipEntries(ctx, data)
	if err != nil {
		return nil, err
	}
	if d.zip64 && uint64(count) != d.records || uint16(count) != uint16(d.records) {
		return nil, zip.ErrFormat
	}
	reader, err := zip.NewReader(r, size)
	if err != nil {
		return nil, err
	}
	if len(reader.File) != count {
		return nil, zip.ErrFormat
	}
	return reader, nil
}

type zipDirectory struct {
	records, size, offset uint64
	end                   int64
	zip64                 bool
}

func readZipDirectory(r *zipSnapshot, size int64) (zipDirectory, error) {
	var d zipDirectory
	var footer []byte
	for _, length := range []int64{1024, 65 * 1024} {
		length = min(length, size)
		tail, err := r.capture(size-length, int(length))
		if err != nil {
			return d, err
		}
		for i := len(tail) - 22; i >= 0; i-- {
			if binary.LittleEndian.Uint32(tail[i:]) != zipEndSig {
				continue
			}
			if i+22+int(binary.LittleEndian.Uint16(tail[i+20:])) > len(tail) {
				break
			}
			footer, d.end = tail[i:], size-length+int64(i)
			break
		}
		if footer != nil || length == size {
			break
		}
	}
	if footer == nil {
		return d, zip.ErrFormat
	}
	b := binary.LittleEndian
	if b.Uint16(footer[4:]) != 0 || b.Uint16(footer[6:]) != 0 || b.Uint16(footer[8:]) != b.Uint16(footer[10:]) {
		return d, zip.ErrFormat
	}
	d.records, d.size, d.offset = uint64(b.Uint16(footer[10:])), uint64(b.Uint32(footer[12:])), uint64(b.Uint32(footer[16:]))
	if d.records != 0xffff && d.size != 0xffffffff && d.offset != 0xffffffff || d.end < 20 {
		return d, nil
	}
	locator, err := r.capture(d.end-20, 20)
	if err != nil {
		return d, err
	}
	if b.Uint32(locator) != zip64LocatorSig {
		return d, nil
	}
	if b.Uint32(locator[4:]) != 0 || b.Uint32(locator[16:]) != 1 {
		return d, zip.ErrFormat
	}
	offset := b.Uint64(locator[8:])
	if d.end < 76 || offset > uint64(d.end-76) {
		return d, zip.ErrFormat
	}
	end, err := r.capture(int64(offset), 56)
	if err != nil {
		return d, err
	}
	if b.Uint32(end) != zip64EndSig || b.Uint64(end[4:]) < 44 || b.Uint64(end[4:]) > uint64(d.end-32)-offset || b.Uint32(end[16:]) != 0 || b.Uint32(end[20:]) != 0 || b.Uint64(end[24:]) != b.Uint64(end[32:]) {
		return d, zip.ErrFormat
	}
	d.records, d.size, d.offset = b.Uint64(end[32:]), b.Uint64(end[40:]), b.Uint64(end[48:])
	d.end, d.zip64 = int64(offset), true
	return d, nil
}

func countZipEntries(ctx context.Context, data []byte) (int, error) {
	count := 0
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if len(data) >= 6 && binary.LittleEndian.Uint32(data) == 0x05054b50 && int(binary.LittleEndian.Uint16(data[4:]))+6 == len(data) {
			break
		}
		if len(data) < 46 || binary.LittleEndian.Uint32(data) != zipDirectorySig {
			return 0, zip.ErrFormat
		}
		length := 46 + int(binary.LittleEndian.Uint16(data[28:])) + int(binary.LittleEndian.Uint16(data[30:])) + int(binary.LittleEndian.Uint16(data[32:]))
		if length > len(data) {
			return 0, zip.ErrFormat
		}
		count++
		if count > maxZipFiles {
			return 0, errors.New(locales.ArchiveDirectorySizeOrEntryCountExceedsTheLimit())
		}
		data = data[length:]
	}
	return count, nil
}

type zipRegion struct {
	offset int64
	data   []byte
}

type zipSnapshot struct {
	source  io.ReaderAt
	ctx     context.Context
	regions []zipRegion
}

func (r *zipSnapshot) capture(offset int64, length int) ([]byte, error) {
	data := make([]byte, length)
	for n := 0; n < len(data); {
		if err := r.ctx.Err(); err != nil {
			return nil, err
		}
		chunk := data[n:min(n+(64<<10), len(data))]
		if count, err := r.source.ReadAt(chunk, offset+int64(n)); err != nil {
			return nil, err
		} else if count != len(chunk) {
			return nil, io.ErrUnexpectedEOF
		}
		n += len(chunk)
	}
	for _, region := range r.regions {
		start, end := max(offset, region.offset), min(offset+int64(length), region.offset+int64(len(region.data)))
		if start < end {
			copy(data[start-offset:end-offset], region.data[start-region.offset:end-region.offset])
		}
	}
	r.regions = append(r.regions, zipRegion{offset: offset, data: data})
	return data, nil
}

func (r *zipSnapshot) ReadAt(p []byte, offset int64) (int, error) {
	if r.ctx == nil {
		return r.source.ReadAt(p, offset)
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n := 0
	for len(p) > 0 {
		copied := 0
		for _, region := range r.regions {
			if offset >= region.offset && offset-region.offset < int64(len(region.data)) {
				copied = copy(p, region.data[offset-region.offset:])
				break
			}
		}
		if copied == 0 {
			return n, io.EOF
		}
		n, offset, p = n+copied, offset+int64(copied), p[copied:]
	}
	return n, nil
}
