package instancecontent

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

func (f File) MatchesScreenshot(info os.FileInfo) bool {
	return f.info != nil && info != nil && info.Mode().IsRegular() && os.SameFile(f.info, info) && f.info.Size() == info.Size() && f.info.ModTime().Equal(info.ModTime())
}

func OpenScreenshot(content string, source File) (*os.File, error) {
	if err := fsutil.ValidateID(source.Name); err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(source.Name)) {
	case ".png", ".jpg", ".jpeg", ".webp":
	default:
		return nil, errors.New(locales.InvalidScreenshot())
	}
	if filepath.Clean(source.Path) != filepath.Join(content, "screenshots", source.Name) {
		return nil, errors.New(locales.InvalidScreenshot())
	}
	root, err := os.OpenRoot(content)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat("screenshots")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New(locales.InvalidScreenshotFolder())
	}
	dir, err := root.OpenRoot("screenshots")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	f, err := fsutil.OpenRootRegular(dir, source.Name)
	if err != nil {
		return nil, err
	}
	current, err := f.Stat()
	if err == nil && !source.MatchesScreenshot(current) {
		err = errors.New(locales.ScreenshotChangedRefreshAndTryAgain())
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
