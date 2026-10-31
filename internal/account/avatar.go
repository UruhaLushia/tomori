package account

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/UruhaLushia/tomori/internal/download"
	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

// Avatar loads a skin's face and hat layer. Offline and unavailable skins use
// a local pixel portrait; a network failure can still return a cached image.
func (c *Client) Avatar(ctx context.Context, cacheDir string, a Account) (image.Image, error) {
	fallback := DefaultAvatar(a)
	if a.Kind != Microsoft && a.Kind != Yggdrasil {
		return fallback, nil
	}
	if !validUUID(a.UUID) {
		return fallback, errors.New(locales.InvalidPlayerUuid())
	}
	sum := sha256.Sum256([]byte(a.ID + "\x00" + a.UUID + "\x00" + a.SkinURL))
	path := filepath.Join(cacheDir, download.AvatarCache, fmt.Sprintf("%x.png", sum))
	if cached, fresh := c.cachedAvatar(ctx, path); cached != nil {
		fallback = cached
		if fresh {
			return cached, nil
		}
	}
	if err := ctx.Err(); err != nil {
		return fallback, err
	}
	address := a.SkinURL
	if address == "" {
		var profile struct {
			Properties []struct{ Name, Value string }
		}
		endpoint := "https://sessionserver.mojang.com/session/minecraft/profile/" + a.UUID
		if a.Kind == Yggdrasil {
			if err := a.Validate(); err != nil {
				return fallback, err
			}
			endpoint = a.ServerURL + "sessionserver/session/minecraft/profile/" + a.UUID
		}
		if err := c.request(ctx, http.MethodGet, endpoint, "", nil, &profile); err != nil {
			return fallback, err
		}
		for _, property := range profile.Properties {
			if property.Name != "textures" {
				continue
			}
			if len(property.Value) > 64<<10 {
				return fallback, errors.New(locales.InvalidAuthenticationServerResponse())
			}
			raw, err := base64.StdEncoding.DecodeString(property.Value)
			if err != nil {
				return fallback, err
			}
			var textures struct {
				Textures struct{ Skin struct{ URL string } }
			}
			if err := json.Unmarshal(raw, &textures); err != nil {
				return fallback, err
			}
			address = textures.Textures.Skin.URL
			break
		}
	}
	if address == "" {
		return fallback, nil
	}
	var raw []byte
	var err error
	if a.Kind == Yggdrasil {
		raw, err = c.externalTexture(ctx, a, address)
	} else {
		raw, err = c.Texture(ctx, address)
	}
	if err != nil {
		return fallback, err
	}
	avatar, err := SkinAvatar(raw)
	if err != nil {
		return fallback, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, avatar); err != nil {
		return avatar, err
	}
	if err := ctx.Err(); err != nil {
		return avatar, err
	}
	if c.budget != nil {
		release, err := c.budget.FilePermit(ctx)
		if err != nil {
			return avatar, err
		}
		defer release()
	}
	return avatar, fsutil.WriteAtomic(path, out.Bytes())
}

func (c *Client) cachedAvatar(ctx context.Context, path string) (image.Image, bool) {
	if c.budget != nil {
		release, err := c.budget.FilePermit(ctx)
		if err != nil {
			return nil, false
		}
		defer release()
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, false
	}
	defer root.Close()
	raw, err := fsutil.ReadRootFile(root, filepath.Base(path), 64<<10)
	if err != nil {
		return nil, false
	}
	info, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil || info.Width != 64 || info.Height != 64 {
		return nil, false
	}
	cached, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, false
	}
	stat, err := root.Stat(filepath.Base(path))
	return cached, err == nil && time.Since(stat.ModTime()) < 24*time.Hour
}

// SkinAvatar accepts modern, legacy and scaled Minecraft PNG skins.
func SkinAvatar(raw []byte) (image.Image, error) {
	skin, err := decodeSkin(raw)
	if err != nil {
		return nil, err
	}
	scale := skin.Bounds().Dx() / 64
	face := image.NewNRGBA(image.Rect(0, 0, 8*scale, 8*scale))
	draw.Draw(face, face.Bounds(), skin, image.Pt(8*scale, 8*scale), draw.Src)
	if skinHatVisible(skin) {
		draw.Draw(face, face.Bounds(), skin, image.Pt(40*scale, 8*scale), draw.Over)
	}
	return enlargeFace(face), nil
}

func enlargeFace(face image.Image) image.Image {
	out := image.NewRGBA(image.Rect(0, 0, 64, 64))
	xdraw.NearestNeighbor.Scale(out, out.Bounds(), face, face.Bounds(), draw.Src, nil)
	return out
}

func DefaultAvatar(a Account) image.Image {
	face := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	palette := []color.NRGBA{{R: 0x32, G: 0x24, B: 0x1c, A: 255}, {R: 0xb7, G: 0x84, B: 0x61, A: 255}, {R: 0xcf, G: 0xa5, B: 0x7d, A: 255}, {R: 0xf0, G: 0xe8, B: 0xd4, A: 255}, {R: 0x50, G: 0x5b, B: 0x96, A: 255}, {R: 0x6b, G: 0x43, B: 0x30, A: 255}}
	if len(a.UUID) > 0 && a.UUID[len(a.UUID)-1]%2 == 1 {
		palette[0] = color.NRGBA{R: 0x9a, G: 0x4f, B: 0x27, A: 255}
		palette[4] = color.NRGBA{R: 0x4e, G: 0x6c, B: 0x45, A: 255}
	}
	for y, row := range []string{"00000000", "00000000", "01122110", "12222221", "13422431", "12211221", "12555521", "11255211"} {
		for x, pixel := range row {
			face.SetNRGBA(x, y, palette[pixel-'0'])
		}
	}
	return enlargeFace(face)
}
