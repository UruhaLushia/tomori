package fsutil

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/UruhaLushia/tomori/locales"
)

func PublishRootExclusive(ctx context.Context, root *os.Root, source, target string, expected os.FileInfo) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := copyPath(source); err != nil {
		return err
	}
	if err := ValidateID(target); err != nil {
		return err
	}
	in, err := OpenRootRegular(root, source)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if expected != nil && !sameCopyFile(expected, before) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	if err := root.Link(source, target); err == nil {
		current, err := root.Lstat(target)
		if err != nil {
			return err
		}
		if !sameCopyFile(before, current) {
			return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
		return nil
	} else if errors.Is(err, os.ErrExist) {
		return err
	}
	out, err := root.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	info, err := out.Stat()
	if err != nil {
		_ = out.Close()
		return err
	}
	keep := false
	defer func() {
		if !keep {
			current, e := root.Lstat(target)
			if e == nil && os.SameFile(info, current) {
				_ = root.Remove(target)
			}
		}
	}()
	n, err := io.CopyBuffer(out, io.LimitReader(contextReader{ctx, in}, before.Size()+1), make([]byte, 64<<10))
	if err == nil {
		after, e := in.Stat()
		if e != nil {
			err = e
		} else if n != before.Size() || !sameCopyFile(before, after) {
			err = errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
		}
	}
	if err == nil {
		err = out.Sync()
	}
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	keep = true
	return nil
}
