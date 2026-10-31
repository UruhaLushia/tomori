package fsutil

import (
	"context"
	"crypto/sha1"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/UruhaLushia/tomori/locales"
)

func ValidateID(id string) error {
	if id == "" || !utf8.ValidString(id) || id == "." || id == ".." || strings.ContainsAny(id, "/\\:\x00<>\"|?*") || strings.TrimSpace(id) != id || strings.HasSuffix(id, ".") {
		return fmt.Errorf(locales.InvalidVersionOrResourceId(), id)
	}
	for _, r := range id {
		if r < 32 || r == 127 {
			return fmt.Errorf(locales.InvalidVersionOrResourceId(), id)
		}
	}
	base, _, _ := strings.Cut(id, ".")
	base = strings.ToUpper(strings.TrimRight(base, " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return fmt.Errorf(locales.InvalidVersionOrResourceId(), id)
	}
	if strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT") {
		suffix := base[3:]
		if suffix != "" && strings.Contains("|1|2|3|4|5|6|7|8|9|¹|²|³|", "|"+suffix+"|") {
			return fmt.Errorf(locales.InvalidVersionOrResourceId(), id)
		}
	}
	return nil
}

func SafePath(root, name string) (string, error) {
	name = strings.ReplaceAll(name, "\\", "/")
	if !filepath.IsLocal(filepath.FromSlash(name)) || strings.Contains(name, ":") {
		return "", fmt.Errorf(locales.InvalidResourcePath2(), name)
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

func WriteAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tomori-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func Complete(path, sum string, size int64) bool {
	return CompleteHashesContext(context.Background(), path, sum, "", size)
}

var hashBuffers = sync.Pool{New: func() any { b := make([]byte, 64<<10); return &b }}

func CompleteRootContext(ctx context.Context, root *os.Root, name, sum string, size int64) bool {
	if ctx.Err() != nil {
		return false
	}
	f, err := OpenRootRegular(root, name)
	if err != nil {
		return false
	}
	defer f.Close()
	return completeFileHashesContext(ctx, f, sum, "", size)
}

func CompleteHashesContext(ctx context.Context, path, sum, sum512 string, size int64) bool {
	if ctx.Err() != nil {
		return false
	}
	f, err := OpenRegular(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	return completeFileHashesContext(ctx, f, sum, sum512, size)
}

func completeFileHashesContext(ctx context.Context, f *os.File, sum, sum512 string, size int64) bool {
	if ctx.Err() != nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	if size > 0 && info.Size() != size {
		return false
	}
	if sum == "" && sum512 == "" {
		return info.Size() > 0 || size > 0
	}
	var h, h512 hash.Hash
	var digest io.Writer
	if sum != "" {
		h = sha1.New()
		digest = h
	}
	if sum512 != "" {
		h512 = sha512.New()
		digest = h512
		if sum != "" {
			digest = io.MultiWriter(h, h512)
		}
	}
	buffer := hashBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(digest, io.LimitReader(contextReader{ctx, f}, info.Size()+1), *buffer)
	hashBuffers.Put(buffer)
	if err != nil {
		return false
	}
	after, err := f.Stat()
	if err != nil || n != info.Size() || !sameCopyFile(info, after) {
		return false
	}
	return ctx.Err() == nil && (sum == "" || strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum)) && (sum512 == "" || strings.EqualFold(hex.EncodeToString(h512.Sum(nil)), sum512))
}
