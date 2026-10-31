package resourcecatalog

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/UruhaLushia/tomori/internal/resource"
	"github.com/UruhaLushia/tomori/locales"
)

type Fingerprint struct {
	Value   uint32
	SHA1    string
	Enabled bool
	Size    int64
}

func EntryFingerprint(ctx context.Context, gameDir string, kind resource.Kind, entry resource.Entry) (Fingerprint, error) {
	f, err := resource.OpenEntry(gameDir, kind, entry)
	if err != nil {
		return Fingerprint{}, err
	}
	defer f.Close()
	result, err := FingerprintFile(ctx, f)
	result.Enabled = entry.Enabled
	return result, err
}

func FingerprintFile(ctx context.Context, f *os.File) (Fingerprint, error) {
	info, err := f.Stat()
	if err != nil {
		return Fingerprint{}, err
	}
	if !info.Mode().IsRegular() {
		return Fingerprint{}, errors.New(locales.SelectResourceFiles())
	}
	var buffer [64 << 10]byte
	var length uint64
	for {
		if err := ctx.Err(); err != nil {
			return Fingerprint{}, err
		}
		n, err := f.Read(buffer[:])
		for _, b := range buffer[:n] {
			if fingerprintByte(b) {
				length++
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Fingerprint{}, err
		}
	}
	if length > 1<<32-1 {
		return Fingerprint{}, errors.New(locales.ResourceFileExceedsTheFingerprintSizeLimit())
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Fingerprint{}, err
	}
	const m uint32 = 0x5bd1e995
	h := uint32(1) ^ uint32(length)
	var word uint32
	var shift uint
	hash := sha1.New()
	for {
		if err := ctx.Err(); err != nil {
			return Fingerprint{}, err
		}
		n, err := f.Read(buffer[:])
		_, _ = hash.Write(buffer[:n])
		for _, b := range buffer[:n] {
			if !fingerprintByte(b) {
				continue
			}
			word |= uint32(b) << shift
			shift += 8
			if shift == 32 {
				word *= m
				word ^= word >> 24
				word *= m
				h = h*m ^ word
				word, shift = 0, 0
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return Fingerprint{}, err
		}
	}
	if shift != 0 {
		h ^= word
		h *= m
	}
	h ^= h >> 13
	h *= m
	h ^= h >> 15
	after, err := f.Stat()
	if err != nil {
		return Fingerprint{}, err
	}
	if info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return Fingerprint{}, errors.New(locales.ResourceChangedRefreshTheList())
	}
	return Fingerprint{Value: h, SHA1: hex.EncodeToString(hash.Sum(nil)), Size: info.Size()}, nil
}
func fingerprintByte(b byte) bool { return b != ' ' && b != '\t' && b != '\r' && b != '\n' }
