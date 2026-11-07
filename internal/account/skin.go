package account

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"

	"github.com/UruhaLushia/tomori/internal/cache"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

const skinLimit = 4 << 20

func skinConfig(raw []byte) (image.Config, error) {
	if len(raw) > skinLimit {
		return image.Config{}, errors.New(locales.SkinFileTooLarge())
	}
	info, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return info, errors.New(locales.InvalidPngSkin())
	}
	if info.Width < 64 || info.Width > 1024 || info.Width%64 != 0 || info.Height != info.Width && info.Height*2 != info.Width {
		return info, errors.New(locales.InvalidSkinDimensions())
	}
	return info, nil
}

func decodeSkin(raw []byte) (image.Image, error) {
	if _, err := skinConfig(raw); err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New(locales.InvalidPngSkin())
	}
	return img, nil
}

// DetectSkinVariant follows the same unused-arm-area rule used by skinview3d:
// transparent, solid-black or solid-white slim-only pixels identify Alex arms.
func DetectSkinVariant(raw []byte) (string, error) {
	skin, err := decodeSkin(raw)
	if err != nil {
		return "", err
	}
	if skin.Bounds().Dy()*2 == skin.Bounds().Dx() {
		return "classic", nil
	}
	scale := skin.Bounds().Dx() / 64
	areas := [4][4]int{{50, 16, 2, 4}, {54, 20, 2, 12}, {42, 48, 2, 4}, {46, 52, 2, 12}}
	transparent, black, white := false, true, true
	for _, area := range areas {
		for y := area[1] * scale; y < (area[1]+area[3])*scale; y++ {
			for x := area[0] * scale; x < (area[0]+area[2])*scale; x++ {
				pixel := color.NRGBAModel.Convert(skin.At(x, y)).(color.NRGBA)
				transparent = transparent || pixel.A != 0xff
				black = black && pixel.R == 0 && pixel.G == 0 && pixel.B == 0 && pixel.A == 0xff
				white = white && pixel.R == 0xff && pixel.G == 0xff && pixel.B == 0xff && pixel.A == 0xff
			}
		}
	}
	if transparent || black || white {
		return "slim", nil
	}
	return "classic", nil
}

func ReadSkinFile(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	raw, err := fsutil.ReadRootFile(root, filepath.Base(path), skinLimit)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return raw, nil
}

func textureURL(address string) (string, error) {
	u, err := url.Parse(address)
	if err != nil || u.Hostname() != "textures.minecraft.net" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" || !strings.HasPrefix(u.Path, "/texture/") {
		return "", errors.New(locales.InvalidPlayerSkinUrl())
	}
	u.Scheme = "https"
	return u.String(), nil
}

func (c *Client) Texture(ctx context.Context, address string) ([]byte, error) {
	address, err := textureURL(address)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	release, err := c.network(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(locales.FailedToReadSkin(), resp.Status)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, skinLimit+1))
	if err != nil {
		return nil, err
	}
	if _, err := skinConfig(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// TextureCached keeps immutable Minecraft texture URLs on disk. Texture URLs
// include the content hash, so a cache entry never needs a time based expiry.
func (c *Client) TextureCached(ctx context.Context, cacheDir, address string) ([]byte, error) {
	address, err := textureURL(address)
	if err != nil {
		return nil, err
	}
	path := cache.File(cacheDir, address)
	if raw, ok := readTextureCache(ctx, path); ok {
		return raw, nil
	}
	raw, err := c.Texture(ctx, address)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
		_ = fsutil.WriteAtomic(path, raw)
	}
	return raw, nil
}

func readTextureCache(ctx context.Context, path string) ([]byte, bool) {
	if err := ctx.Err(); err != nil {
		return nil, false
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, false
	}
	defer root.Close()
	raw, err := fsutil.ReadRootFile(root, filepath.Base(path), skinLimit)
	if err != nil {
		return nil, false
	}
	if _, err := skinConfig(raw); err != nil {
		return nil, false
	}
	return raw, true
}

func ExportSkin(ctx context.Context, directory, name string, raw []byte) (string, error) {
	if _, err := decodeSkin(raw); err != nil {
		return "", err
	}
	if err := fsutil.ValidateID(name); err != nil {
		return "", err
	}
	if strings.ToLower(filepath.Ext(name)) != ".png" {
		return "", errors.New(locales.InvalidPngSkin())
	}
	report, err := storage.Assess(ctx, []storage.Requirement{{Directory: directory, Bytes: uint64(len(raw)) * 2}})
	if err != nil {
		return "", err
	}
	if err := report.Check(); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	temp := ".tomori-skin-" + rand.Text()
	if err := fsutil.WriteRootAtomic(ctx, root, temp, raw); err != nil {
		return "", err
	}
	defer func() { _ = root.Remove(temp) }()
	for i := range 1000 {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d).png", strings.TrimSuffix(name, filepath.Ext(name)), i)
		}
		err = fsutil.PublishRootExclusive(ctx, root, temp, candidate, nil)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return filepath.Join(directory, candidate), nil
	}
	return "", errors.New(locales.TooManyFilesWithTheSameName())
}

func SkinPreview(raw []byte, variant string) ([2]image.Image, error) {
	var views [2]image.Image
	if variant != "classic" && variant != "slim" {
		return views, errors.New(locales.InvalidSkinModel())
	}
	skin, err := decodeSkin(raw)
	if err != nil {
		return views, err
	}
	scale := skin.Bounds().Dx() / 64
	legacy := skin.Bounds().Dy()*2 == skin.Bounds().Dx()
	hat := skinHatVisible(skin)
	if legacy {
		skin = expandLegacySkin(skin)
	}
	arm := 4
	if variant == "slim" {
		arm = 3
	}
	for side := range 2 {
		canvas := image.NewRGBA(image.Rect(0, 0, 16*scale, 32*scale))
		tile := func(x, y, w, h, dx, dy int) {
			r := image.Rect(dx*scale, dy*scale, (dx+w)*scale, (dy+h)*scale)
			draw.Draw(canvas, r, skin, image.Pt(x*scale, y*scale), draw.Over)
		}
		if side == 0 {
			tile(8, 8, 8, 8, 4, 0)
			tile(20, 20, 8, 12, 4, 8)
			tile(44, 20, arm, 12, 4-arm, 8)
			tile(4, 20, 4, 12, 4, 20)
			tile(36, 52, arm, 12, 12, 8)
			tile(20, 52, 4, 12, 8, 20)
			if !legacy {
				tile(20, 36, 8, 12, 4, 8)
				tile(44, 36, arm, 12, 4-arm, 8)
				tile(52, 52, arm, 12, 12, 8)
				tile(4, 36, 4, 12, 4, 20)
				tile(4, 52, 4, 12, 8, 20)
			}
			if hat {
				tile(40, 8, 8, 8, 4, 0)
			}
		} else {
			tile(24, 8, 8, 8, 4, 0)
			tile(32, 20, 8, 12, 4, 8)
			tile(44+arm+4, 20, arm, 12, 12, 8)
			tile(12, 20, 4, 12, 8, 20)
			tile(36+arm+4, 52, arm, 12, 4-arm, 8)
			tile(28, 52, 4, 12, 4, 20)
			if !legacy {
				tile(32, 36, 8, 12, 4, 8)
				tile(44+arm+4, 36, arm, 12, 12, 8)
				tile(52+arm+4, 52, arm, 12, 4-arm, 8)
				tile(12, 36, 4, 12, 8, 20)
				tile(12, 52, 4, 12, 4, 20)
			}
			if hat {
				tile(56, 8, 8, 8, 4, 0)
			}
		}
		out := image.NewRGBA(image.Rect(0, 0, 128, 256))
		xdraw.NearestNeighbor.Scale(out, out.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
		views[side] = out
	}
	return views, nil
}

func expandLegacySkin(skin image.Image) image.Image {
	scale := skin.Bounds().Dx() / 64
	out := image.NewNRGBA(image.Rect(0, 0, 64*scale, 64*scale))
	pixels := skin.(image.RGBA64Image)
	draw.Draw(out, skin.Bounds(), skin, image.Point{}, draw.Src)
	for _, face := range [][6]int{{4, 20, 4, 12, 20, 52}, {12, 20, 4, 12, 28, 52}, {8, 20, 4, 12, 16, 52}, {0, 20, 4, 12, 24, 52}, {4, 16, 4, 4, 20, 48}, {8, 16, 4, 4, 24, 48}, {44, 20, 4, 12, 36, 52}, {52, 20, 4, 12, 44, 52}, {48, 20, 4, 12, 32, 52}, {40, 20, 4, 12, 40, 52}, {44, 16, 4, 4, 36, 48}, {48, 16, 4, 4, 40, 48}} {
		x, y, w, h, dx, dy := face[0], face[1], face[2], face[3], face[4], face[5]
		for yy := range h * scale {
			for xx := range w * scale {
				out.SetRGBA64(dx*scale+xx, dy*scale+yy, pixels.RGBA64At((x+w)*scale-xx-1, y*scale+yy))
			}
		}
	}
	return out
}

func skinHatVisible(skin image.Image) bool {
	scale := skin.Bounds().Dx() / 64
	if skin.Bounds().Dy() == skin.Bounds().Dx() {
		return true
	}
	pixels := skin.(image.RGBA64Image)
	for y := 0; y < 32*scale; y++ {
		for x := 32 * scale; x < 64*scale; x++ {
			if pixels.RGBA64At(x, y).A < 0x8000 {
				return true
			}
		}
	}
	return false
}

func CapePreview(raw []byte) (image.Image, error) {
	skin, err := decodeSkin(raw)
	if err != nil {
		return nil, err
	}
	if skin.Bounds().Dy()*2 != skin.Bounds().Dx() {
		return nil, errors.New(locales.InvalidCapeDimensions())
	}
	scale := skin.Bounds().Dx() / 64
	out := image.NewRGBA(image.Rect(0, 0, 80, 128))
	xdraw.NearestNeighbor.Scale(out, out.Bounds(), skin, image.Rect(scale, scale, 11*scale, 17*scale), draw.Src, nil)
	return out, nil
}
