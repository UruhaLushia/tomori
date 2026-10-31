package fsutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/UruhaLushia/tomori/locales"
)

func WalkFiles(ctx context.Context, root *os.Root, include func(string, bool) bool, visit func(string, os.FileInfo) error) error {
	entries := 0
	var walk func(string, int) error
	walk = func(path string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 128 {
			return errors.New(locales.InstanceDirectoryNestingIsTooDeep())
		}
		dir, err := root.OpenRoot(path)
		if err != nil {
			return err
		}
		defer dir.Close()
		f, err := dir.Open(".")
		if err != nil {
			return err
		}
		defer f.Close()
		before, err := f.Stat()
		if err != nil {
			return err
		}
		for {
			batch, readErr := f.ReadDir(128)
			for _, entry := range batch {
				if err := ctx.Err(); err != nil {
					return err
				}
				name := filepath.Join(path, entry.Name())
				if include != nil && !include(name, entry.IsDir()) {
					continue
				}
				if err := copyPath(name); err != nil {
					return err
				}
				entries++
				if entries > 200000 {
					return errors.New(locales.ExportFileCountOrSizeExceedsTheLimit())
				}
				if entry.IsDir() {
					if err := walk(name, depth+1); err != nil {
						return err
					}
					continue
				}
				info, err := root.Lstat(name)
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf(locales.CannotCopyALinkOrSpecialFile(), name)
				}
				if err := visit(name, info); err != nil {
					return err
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		after, err := root.Lstat(path)
		if err != nil {
			return err
		}
		if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		return nil
	}
	return walk(".", 0)
}
