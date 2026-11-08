package minecraft

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	imageDraw "image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"time"

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
	info, icon, err := decodeIcon(raw)
	if err != nil {
		return nil, err
	}
	if info.Width <= 0 || info.Height <= 0 || info.Width > 2048 || info.Height > 2048 {
		return nil, errors.New(locales.InvalidInstanceIconDimensions())
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

func AnimatedIcon(raw []byte) ([]byte, bool) {
	if len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WEBP" {
		if animatedWebP(raw) {
			return raw, true
		}
		return nil, false
	}
	if len(raw) >= 6 && (string(raw[:6]) == "GIF87a" || string(raw[:6]) == "GIF89a") {
		return raw, true
	}
	return nil, false
}

type AnimatedWebPFrame struct {
	Data     []byte
	Duration time.Duration
	X, Y     int
	Width    int
	Height   int
	Blend    bool
	Dispose  bool
}

type AnimatedWebP struct {
	Width, Height int
	Frames        []AnimatedWebPFrame
}

func ParseAnimatedWebP(raw []byte) (AnimatedWebP, bool) {
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WEBP" {
		return AnimatedWebP{}, false
	}
	width, height := 0, 0
	var frames []AnimatedWebPFrame
	for offset := 12; offset+8 <= len(raw); {
		tag := string(raw[offset : offset+4])
		size := int(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		start, end := offset+8, offset+8+size
		if size < 0 || start > len(raw) || size > len(raw)-start {
			return AnimatedWebP{}, false
		}
		if tag == "VP8X" && size >= 10 {
			width = 1 + read24(raw[start+4:start+7])
			height = 1 + read24(raw[start+7:start+10])
		}
		if tag == "ANMF" && size > 16 {
			payload := raw[start:end]
			data, ok := webpFrameData(payload)
			if !ok {
				return AnimatedWebP{}, false
			}
			x := 2 * read24(payload[0:3])
			y := 2 * read24(payload[3:6])
			frameWidth := 1 + read24(payload[6:9])
			frameHeight := 1 + read24(payload[9:12])
			milliseconds := read24(payload[12:15])
			var duration time.Duration
			if milliseconds > 0 {
				duration = time.Duration(milliseconds) * time.Millisecond
			}
			flags := payload[15]
			frames = append(frames, AnimatedWebPFrame{
				Data: data, Duration: duration, X: x, Y: y,
				Width: frameWidth, Height: frameHeight,
				Blend: flags&2 == 0, Dispose: flags&1 != 0,
			})
		}
		offset = end + size&1
	}
	if len(frames) < 2 || width <= 0 || height <= 0 || width > 2048 || height > 2048 {
		return AnimatedWebP{}, false
	}
	for _, frame := range frames {
		if frame.Width <= 0 || frame.Height <= 0 || frame.X < 0 || frame.Y < 0 || frame.X > width-frame.Width || frame.Y > height-frame.Height {
			return AnimatedWebP{}, false
		}
	}
	return AnimatedWebP{Width: width, Height: height, Frames: frames}, true
}

func decodeIcon(raw []byte) (image.Config, image.Image, error) {
	info, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err == nil {
		icon, _, decodeErr := image.Decode(bytes.NewReader(raw))
		if decodeErr == nil {
			return info, icon, nil
		}
		err = decodeErr
	}
	animation, ok := ParseAnimatedWebP(raw)
	if !ok {
		return image.Config{}, nil, err
	}
	frame := animation.Frames[0]
	icon, _, decodeErr := image.Decode(bytes.NewReader(frame.Data))
	if decodeErr != nil {
		return image.Config{}, nil, decodeErr
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, animation.Width, animation.Height))
	imageDraw.Draw(canvas, image.Rect(frame.X, frame.Y, frame.X+frame.Width, frame.Y+frame.Height), icon, icon.Bounds().Min, imageDraw.Over)
	return image.Config{Width: animation.Width, Height: animation.Height}, canvas, nil
}

func animatedWebP(raw []byte) bool {
	_, ok := ParseAnimatedWebP(raw)
	return ok
}

func webpFrameData(payload []byte) ([]byte, bool) {
	if len(payload) < 16 {
		return nil, false
	}
	width := 1 + read24(payload[6:9])
	height := 1 + read24(payload[9:12])
	var chunks []byte
	alpha := false
	imageChunk := false
	for offset := 16; offset+8 <= len(payload); {
		tag := payload[offset : offset+4]
		size := int(binary.LittleEndian.Uint32(payload[offset+4 : offset+8]))
		end := offset + 8 + size
		if size < 0 || end > len(payload) {
			return nil, false
		}
		if string(tag) == "ALPH" {
			alpha = true
		}
		if string(tag) == "ALPH" || string(tag) == "VP8 " || string(tag) == "VP8L" {
			chunks = append(chunks, payload[offset:end]...)
			imageChunk = true
			if size&1 != 0 {
				chunks = append(chunks, 0)
			}
		}
		offset = end + size&1
	}
	if width <= 0 || height <= 0 || !imageChunk {
		return nil, false
	}
	flags := byte(0)
	if alpha {
		flags = 0x10
	}
	vp8x := make([]byte, 10)
	vp8x[0] = flags
	write24(vp8x[4:7], width-1)
	write24(vp8x[7:10], height-1)
	body := append(webpChunk("VP8X", vp8x), chunks...)
	out := make([]byte, 12, 12+len(body))
	copy(out[:4], "RIFF")
	copy(out[8:12], "WEBP")
	out = append(out, body...)
	binary.LittleEndian.PutUint32(out[4:8], uint32(len(out)-8))
	return out, true
}

func webpChunk(tag string, payload []byte) []byte {
	chunk := make([]byte, 8, 8+len(payload)+len(payload)&1)
	copy(chunk, tag)
	binary.LittleEndian.PutUint32(chunk[4:8], uint32(len(payload)))
	chunk = append(chunk, payload...)
	if len(payload)&1 != 0 {
		chunk = append(chunk, 0)
	}
	return chunk
}

func read24(raw []byte) int {
	return int(raw[0]) | int(raw[1])<<8 | int(raw[2])<<16
}

func write24(raw []byte, value int) {
	raw[0] = byte(value)
	raw[1] = byte(value >> 8)
	raw[2] = byte(value >> 16)
}
