package modpack

import (
	"bytes"
	"context"
	"image/png"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/minecraft"
)

// Preserve local pack artwork without extracting launcher settings or accounts.
// Invalid or absent artwork is optional and does not block the pack import.
func (a *Archive) WriteIcon(ctx context.Context, dir string) error {
	var candidates []string
	if raw, err := a.read(ctx, a.prefix+"instance.cfg"); err == nil {
		for line := range strings.SplitSeq(string(raw), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			value = strings.TrimSpace(value)
			if ok && strings.TrimSpace(key) == "iconKey" && fsutil.ValidateID(value) == nil {
				for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
					candidates = append(candidates, a.prefix+"icons/"+value+ext, a.prefix+value+ext)
				}
			}
		}
	}
	for _, prefix := range append([]string{a.prefix}, a.overrides...) {
		for _, name := range []string{"icon.png", "instance.png", "pack.png", "icon.jpg", "icon.jpeg", "icon.webp", "PCL/Logo.png"} {
			candidates = append(candidates, prefix+name)
		}
	}
	for _, name := range candidates {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := a.entries[name]
		if entry == nil || entry.UncompressedSize64 > 8<<20 {
			continue
		}
		raw, err := a.read(ctx, name)
		if err != nil {
			continue
		}
		icon, err := minecraft.DecodeInstanceIcon(raw)
		if err != nil {
			continue
		}
		var out bytes.Buffer
		if err := png.Encode(&out, icon); err != nil {
			return err
		}
		return fsutil.WriteAtomic(filepath.Join(dir, "icon.png"), out.Bytes())
	}
	return nil
}
