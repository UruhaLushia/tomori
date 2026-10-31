package instancecontent

import (
	"context"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func LoadImage(ctx context.Context, path string, maxWidth, maxHeight int) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return loadImage(ctx, f, maxWidth, maxHeight)
}

func LoadScreenshot(ctx context.Context, content string, source File, maxWidth, maxHeight int) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := OpenScreenshot(content, source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := loadImage(ctx, f, maxWidth, maxHeight)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && !source.MatchesScreenshot(info) {
		err = errors.New(locales.ScreenshotChangedRefreshAndTryAgain())
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

func loadImage(ctx context.Context, f *os.File, maxWidth, maxHeight int) (image.Image, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const limit = 32 << 20
	const maxDecodedPixels = (64 << 20) / 4
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return nil, errors.New(locales.ImageFileTooLargeOrInvalid())
	}
	reader := fsutil.ContextReader(ctx, io.LimitReader(f, limit))
	meta, _, err := image.DecodeConfig(reader)
	if err != nil {
		return nil, err
	}
	if meta.Width <= 0 || meta.Height <= 0 || int64(meta.Width)*int64(meta.Height) > maxDecodedPixels {
		return nil, errors.New(locales.ImageDimensionsTooLargeOrInvalid())
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	reader = fsutil.ContextReader(ctx, io.LimitReader(f, limit))
	src, _, err := image.Decode(reader)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	w, h := meta.Width, meta.Height
	if w > maxWidth {
		h, w = max(1, h*maxWidth/w), maxWidth
	}
	if h > maxHeight {
		w, h = max(1, w*maxHeight/h), maxHeight
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	return dst, ctx.Err()
}
