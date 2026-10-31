package fsutil

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

func CopyRootVerified(ctx context.Context, source *os.Root, name string, destination *os.Root, target, sum string, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := copyPath(name); err != nil {
		return err
	}
	if err := copyPath(target); err != nil {
		return err
	}
	in, err := OpenRootRegular(source, name)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if before.Size() != size {
		return fmt.Errorf(locales.SizeVerificationFailed(), name, before.Size(), size)
	}
	dir := filepath.Dir(target)
	if err := destination.MkdirAll(dir, 0755); err != nil {
		return err
	}
	temp := filepath.Join(dir, ".tomori-copy-"+rand.Text())
	out, err := destination.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = destination.Remove(temp) }()
	h := sha1.New()
	buffer := hashBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(io.MultiWriter(out, h), io.LimitReader(contextReader{ctx, in}, size+1), *buffer)
	hashBuffers.Put(buffer)
	if closeErr := out.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	after, err := in.Stat()
	if err != nil {
		return err
	}
	if n != size || !sameCopyFile(before, after) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), sum) {
		return fmt.Errorf(locales.VerificationFailed(), name)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return destination.Rename(temp, target)
}
