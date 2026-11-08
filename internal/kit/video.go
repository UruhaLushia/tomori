package kit

import (
	"bytes"
	"errors"
	"image"
	"image/gif"
	"io"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/UruhaLushia/tomori/internal/minecraft"
	"golang.org/x/image/draw"
)

type webpVideoDecoder struct {
	animation minecraft.AnimatedWebP
	index     int
	canvas    *image.NRGBA
	scale     float64
	width     int
	height    int
}

const videoMaxSize = 64

func AnimatedVideo(raw []byte) (*ui.VideoPlayer, error) {
	if len(raw) >= 6 && (string(raw[:6]) == "GIF87a" || string(raw[:6]) == "GIF89a") {
		decoder, err := newGIFVideoDecoder(raw)
		if err != nil {
			return nil, err
		}
		player, err := ui.NewVideoPlayer(decoder)
		if err != nil {
			return nil, err
		}
		return player.SetLoop(true), nil
	}
	animation, ok := minecraft.ParseAnimatedWebP(raw)
	if !ok {
		return nil, errors.New("unsupported animated image")
	}
	scale := min(1.0, float64(videoMaxSize)/float64(max(animation.Width, animation.Height)))
	decoder := &webpVideoDecoder{
		animation: animation,
		scale:     scale,
		width:     max(1, int(math.Round(float64(animation.Width)*scale))),
		height:    max(1, int(math.Round(float64(animation.Height)*scale))),
	}
	player, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		return nil, err
	}
	return player.SetLoop(true), nil
}

type gifVideoDecoder struct {
	frames    []*image.NRGBA
	durations []time.Duration
	index     int
}

func newGIFVideoDecoder(raw []byte) (*gifVideoDecoder, error) {
	animation, err := gif.DecodeAll(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if len(animation.Image) == 0 {
		return nil, errors.New("unsupported animated image")
	}
	width, height := animation.Config.Width, animation.Config.Height
	if width <= 0 || height <= 0 {
		for _, frame := range animation.Image {
			width = max(width, frame.Bounds().Max.X)
			height = max(height, frame.Bounds().Max.Y)
		}
	}
	scale := min(1.0, float64(videoMaxSize)/float64(max(width, height)))
	targetWidth := max(1, int(math.Round(float64(width)*scale)))
	targetHeight := max(1, int(math.Round(float64(height)*scale)))
	canvas := image.NewNRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	var previous *image.NRGBA
	decoder := &gifVideoDecoder{
		frames:    make([]*image.NRGBA, 0, len(animation.Image)),
		durations: make([]time.Duration, 0, len(animation.Image)),
	}
	for index, frame := range animation.Image {
		if index > 0 {
			switch gifDisposal(animation, index-1) {
			case gif.DisposalBackground:
				disposeGIFFrame(canvas, animation.Image[index-1], scale)
			case gif.DisposalPrevious:
				if previous != nil {
					copy(canvas.Pix, previous.Pix)
				}
			}
		}
		bounds := scaleGIFRect(frame.Bounds(), scale, canvas.Rect)
		if gifDisposal(animation, index) == gif.DisposalPrevious {
			previous = cloneNRGBA(canvas)
		} else {
			previous = nil
		}
		scaled := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
		draw.ApproxBiLinear.Scale(scaled, scaled.Rect, frame, frame.Bounds(), draw.Src, nil)
		draw.Draw(canvas, bounds, scaled, scaled.Bounds().Min, draw.Over)
		decoder.frames = append(decoder.frames, cloneNRGBA(canvas))
		duration := 100 * time.Millisecond
		if index < len(animation.Delay) && animation.Delay[index] > 1 {
			duration = time.Duration(animation.Delay[index]) * 10 * time.Millisecond
		}
		decoder.durations = append(decoder.durations, duration)
	}
	return decoder, nil
}

func (d *gifVideoDecoder) Next() (ui.VideoFrame, error) {
	if d.index >= len(d.frames) {
		return ui.VideoFrame{}, io.EOF
	}
	frame := d.frames[d.index]
	duration := d.durations[d.index]
	d.index++
	return ui.VideoFrame{Image: frame, Duration: duration}, nil
}

func (d *gifVideoDecoder) Rewind() error {
	d.index = 0
	return nil
}

func gifDisposal(animation *gif.GIF, index int) byte {
	if index < len(animation.Disposal) {
		return animation.Disposal[index]
	}
	return gif.DisposalNone
}

func disposeGIFFrame(canvas *image.NRGBA, frame *image.Paletted, scale float64) {
	clearImageRect(canvas, scaleGIFRect(frame.Bounds(), scale, canvas.Rect))
}

func scaleGIFRect(rect image.Rectangle, scale float64, bounds image.Rectangle) image.Rectangle {
	x0 := int(math.Round(float64(rect.Min.X) * scale))
	y0 := int(math.Round(float64(rect.Min.Y) * scale))
	x1 := int(math.Round(float64(rect.Max.X) * scale))
	y1 := int(math.Round(float64(rect.Max.Y) * scale))
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	return image.Rect(x0, y0, x1, y1).Intersect(bounds)
}

func cloneNRGBA(src *image.NRGBA) *image.NRGBA {
	dst := image.NewNRGBA(src.Rect)
	copy(dst.Pix, src.Pix)
	return dst
}

func (d *webpVideoDecoder) Next() (ui.VideoFrame, error) {
	if d.index >= len(d.animation.Frames) {
		return ui.VideoFrame{}, io.EOF
	}
	if d.canvas == nil {
		d.canvas = image.NewNRGBA(image.Rect(0, 0, d.width, d.height))
	}
	if d.index > 0 {
		d.dispose(d.animation.Frames[d.index-1])
	}
	frame := d.animation.Frames[d.index]
	area := d.frameRect(frame)
	if !frame.Blend {
		clearImageRect(d.canvas, area)
	}
	decoded, _, err := image.Decode(bytes.NewReader(frame.Data))
	if err != nil {
		return ui.VideoFrame{}, err
	}
	if area.Dx() != decoded.Bounds().Dx() || area.Dy() != decoded.Bounds().Dy() {
		resized := image.NewNRGBA(image.Rect(0, 0, area.Dx(), area.Dy()))
		draw.ApproxBiLinear.Scale(resized, resized.Rect, decoded, decoded.Bounds(), draw.Src, nil)
		decoded = resized
	}
	draw.Draw(d.canvas, area, decoded, decoded.Bounds().Min, draw.Over)
	d.index++
	return ui.VideoFrame{Image: d.canvas, Duration: frame.Duration}, nil
}

func (d *webpVideoDecoder) Rewind() error {
	d.index = 0
	if d.canvas != nil {
		clear(d.canvas.Pix)
	}
	return nil
}

func (d *webpVideoDecoder) dispose(frame minecraft.AnimatedWebPFrame) {
	if frame.Dispose {
		clearImageRect(d.canvas, d.frameRect(frame))
	}
}

func (d *webpVideoDecoder) frameRect(frame minecraft.AnimatedWebPFrame) image.Rectangle {
	x0 := int(math.Round(float64(frame.X) * d.scale))
	y0 := int(math.Round(float64(frame.Y) * d.scale))
	x1 := int(math.Round(float64(frame.X+frame.Width) * d.scale))
	y1 := int(math.Round(float64(frame.Y+frame.Height) * d.scale))
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	return image.Rect(x0, y0, min(x1, d.width), min(y1, d.height))
}

func clearImageRect(dst *image.NRGBA, rect image.Rectangle) {
	r := rect.Intersect(dst.Rect)
	if r.Empty() {
		return
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		clear(dst.Pix[dst.PixOffset(r.Min.X, y):dst.PixOffset(r.Max.X, y)])
	}
}
