package instancecontent

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/resource"
)

type Count struct{ Total, Enabled int }

type File struct {
	Name, Path    string
	Modified      time.Time
	Size          int64
	Cover         string
	CoverSize     int64
	CoverModified time.Time
	info          os.FileInfo
}

type Summary struct {
	Resources           map[resource.Kind]Count
	Worlds, Screenshots []File
}

// Scan only inspects local directory entries; resource metadata is loaded when
// its management tab is opened. Partial failures preserve the other categories.
func Scan(ctx context.Context, dir string) (Summary, error) {
	out := Summary{Resources: make(map[resource.Kind]Count)}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	root, err := os.OpenRoot(dir)
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	defer func() { _ = root.Close() }()
	var problems []error
	read := func(path string, visit func(fs.DirEntry)) {
		if ctx.Err() != nil {
			return
		}
		f, err := root.Open(filepath.ToSlash(path))
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			problems = append(problems, err)
			return
		}
		defer f.Close()
		for {
			entries, err := f.ReadDir(128)
			for _, entry := range entries {
				if ctx.Err() != nil {
					return
				}
				visit(entry)
			}
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				problems = append(problems, err)
				return
			}
		}
	}
	for _, kind := range []resource.Kind{resource.Mod, resource.ResourcePack, resource.ShaderPack} {
		count := Count{}
		for _, sub := range []string{string(kind), filepath.Join(string(kind), ".tomori-disabled")} {
			read(sub, func(entry fs.DirEntry) {
				if entry.Type()&os.ModeSymlink != 0 || !kind.Accepts(entry.Name(), entry.IsDir()) {
					return
				}
				count.Total++
				if sub == string(kind) && !strings.HasSuffix(entry.Name(), ".disabled") {
					count.Enabled++
				}
			})
		}
		out.Resources[kind] = count
	}
	read("saves", func(entry fs.DirEntry) {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || strings.HasPrefix(entry.Name(), ".tomori-world-") {
			return
		}
		rel := filepath.Join("saves", entry.Name())
		info, err := root.Stat(filepath.Join(rel, "level.dat"))
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			problems = append(problems, err)
			return
		}
		if info.Mode().IsRegular() {
			file := File{Name: entry.Name(), Path: filepath.Join(dir, rel), Modified: info.ModTime(), Size: info.Size()}
			if cover, err := root.Stat(filepath.Join(rel, "icon.png")); err == nil && cover.Mode().IsRegular() {
				file.Cover, file.CoverSize, file.CoverModified = filepath.Join(dir, rel, "icon.png"), cover.Size(), cover.ModTime()
			}
			out.Worlds = append(out.Worlds, file)
		}
	})
	read("screenshots", func(entry fs.DirEntry) {
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".png", ".jpg", ".jpeg", ".webp":
			info, err := entry.Info()
			if err != nil {
				problems = append(problems, err)
				return
			}
			if info.Mode().IsRegular() {
				path := filepath.Join(dir, "screenshots", entry.Name())
				out.Screenshots = append(out.Screenshots, File{Name: entry.Name(), Path: path, Modified: info.ModTime(), Size: info.Size(), Cover: path, CoverSize: info.Size(), CoverModified: info.ModTime(), info: info})
			}
		}
	})
	byTime := func(a, b File) int {
		if n := b.Modified.Compare(a.Modified); n != 0 {
			return n
		}
		return strings.Compare(a.Name, b.Name)
	}
	slices.SortFunc(out.Worlds, byTime)
	slices.SortFunc(out.Screenshots, byTime)
	if err := ctx.Err(); err != nil {
		return out, err
	}
	return out, errors.Join(problems...)
}
