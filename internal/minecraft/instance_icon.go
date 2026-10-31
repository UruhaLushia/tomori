package minecraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func (i Instance) iconCandidates() []string {
	paths := []string{filepath.Join(i.InstanceDir, instanceIconFile)}
	if i.Format == InstancePrism && i.IconKey != "" && fsutil.ValidateID(i.IconKey) == nil {
		for _, root := range []string{i.InstanceDir, filepath.Dir(i.InstanceDir), filepath.Dir(filepath.Dir(i.InstanceDir))} {
			for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
				paths = append(paths, filepath.Join(root, "icons", i.IconKey+ext))
			}
		}
	}
	for _, root := range []string{i.InstanceDir, i.GameDir} {
		for _, name := range []string{"icon.png", "instance.png", "pack.png", "icon.jpg", "icon.jpeg", "icon.webp", "PCL/Logo.png"} {
			paths = append(paths, filepath.Join(root, filepath.FromSlash(name)))
		}
	}
	return paths
}

func InstanceIconSignature(ctx context.Context, instance Instance) ([32]byte, error) {
	var metadata bytes.Buffer
	for _, path := range instance.iconCandidates() {
		if err := ctx.Err(); err != nil {
			return [32]byte{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(&metadata, path, err)
			}
			continue
		}
		fmt.Fprintln(&metadata, path, info.Mode(), info.Size(), info.ModTime().UnixNano())
	}
	return sha256.Sum256(metadata.Bytes()), nil
}

func LoadInstanceIcon(ctx context.Context, instance Instance) (image.Image, error) {
	return loadInstanceIcon(ctx, instance.iconCandidates())
}

func LoadOriginalInstanceIcon(ctx context.Context, instance Instance) (image.Image, error) {
	return loadInstanceIcon(ctx, instance.iconCandidates()[1:])
}

func loadInstanceIcon(ctx context.Context, paths []string) (image.Image, error) {
	var lastErr error
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		icon, err := ReadInstanceIcon(ctx, path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err == nil {
			return icon, nil
		}
		lastErr = fmt.Errorf("%s: %w", path, err)
	}
	return nil, lastErr
}

func ReadInstanceIcon(ctx context.Context, path string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 8<<20 {
		return nil, errors.New(locales.InvalidInstanceIconFileSize())
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 8<<20 {
		return nil, errors.New(locales.InvalidInstanceIconFileSize())
	}
	raw := make([]byte, int(info.Size()))
	if _, err := io.ReadFull(f, raw); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return DecodeInstanceIcon(raw)
}

func DecodeInstanceIcon(raw []byte) (image.Image, error) {
	if len(raw) > 8<<20 {
		return nil, errors.New(locales.InstanceIconTooLarge())
	}
	info, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if info.Width <= 0 || info.Height <= 0 || info.Width > 2048 || info.Height > 2048 {
		return nil, errors.New(locales.InvalidInstanceIconDimensions())
	}
	icon, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	out := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	w, h := 128, 128
	if info.Width > info.Height {
		h = max(1, 128*info.Height/info.Width)
	} else {
		w = max(1, 128*info.Width/info.Height)
	}
	r := image.Rect((128-w)/2, (128-h)/2, (128+w)/2, (128+h)/2)
	draw.CatmullRom.Scale(out, r, icon, icon.Bounds(), draw.Src, nil)
	return out, nil
}
