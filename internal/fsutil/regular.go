package fsutil

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/locales"
)

func OpenRegular(path string) (*os.File, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return OpenRootRegular(root, filepath.Base(path))
}

func OpenRootRegular(root *os.Root, name string) (*os.File, error) {
	return openRootRegular(root, name, true)
}

func OpenRootGrowingFile(root *os.Root, name string) (*os.File, error) {
	return openRootRegular(root, name, false)
}

func openRootRegular(root *os.Root, name string, stable bool) (*os.File, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New(locales.SelectResourceFiles())
	}
	f, err := root.OpenFile(name, os.O_RDONLY|regularOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil && (!info.Mode().IsRegular() || !os.SameFile(before, info) || stable && (before.Size() != info.Size() || !before.ModTime().Equal(info.ModTime()))) {
		err = errors.New(locales.ResourceChangedRefreshTheList())
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
