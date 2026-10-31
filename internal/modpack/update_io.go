package modpack

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/locales"
)

type updateCopy struct {
	info os.FileInfo
	hash string
	size int64
}

func updateContentPath(name string) error {
	first, _, _ := strings.Cut(strings.ToLower(name), "/")
	if name == "" || len(name) > 4096 || strings.Count(name, "/") >= 128 || strings.Contains(name, "\\") || strings.HasPrefix(first, ".tomori-") {
		return invalidOrigin()
	}
	return validContentPath(name)
}

func updateRootCurrent(root *os.Root, expected os.FileInfo) error {
	held, err := root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(root.Name())
	if err != nil {
		return err
	}
	if !held.IsDir() || !current.IsDir() || !os.SameFile(held, current) || expected != nil && !os.SameFile(held, expected) {
		return updateChanged()
	}
	return nil
}

func updateEnsureParents(root *os.Root, name string) error {
	blocked, _, err := updateParents(root, name)
	if err != nil {
		return err
	}
	if blocked {
		return updateChanged()
	}
	if err := root.MkdirAll(path.Dir(name), 0o755); err != nil {
		return err
	}
	blocked, missing, err := updateParents(root, name)
	if err != nil {
		return err
	}
	if blocked || missing {
		return updateChanged()
	}
	return nil
}

func updateHash(ctx context.Context, root *os.Root, name string, expected os.FileInfo) (updateCopy, error) {
	var result updateCopy
	if err := ctx.Err(); err != nil {
		return result, err
	}
	blocked, missing, err := updateParents(root, name)
	if err != nil {
		return result, err
	}
	if blocked || missing {
		return result, updateChanged()
	}
	in, err := fsutil.OpenRootRegular(root, name)
	if err != nil {
		return result, err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return result, err
	}
	if before.Size() > 4<<30 || expected != nil && !sameUpdateFile(expected, before) {
		return result, updateChanged()
	}
	hash := sha1.New()
	buffer := archiveBuffers.Get().(*[]byte)
	n, err := io.CopyBuffer(hash, io.LimitReader(fsutil.ContextReader(ctx, in), before.Size()+1), *buffer)
	archiveBuffers.Put(buffer)
	if err != nil {
		return result, err
	}
	after, err := in.Stat()
	if err != nil {
		return result, err
	}
	latest, err := root.Lstat(name)
	if err != nil {
		return result, err
	}
	if n != before.Size() || !sameUpdateFile(before, after) || !sameUpdateFile(before, latest) {
		return result, updateChanged()
	}
	return updateCopy{info: after, hash: hex.EncodeToString(hash.Sum(nil)), size: n}, ctx.Err()
}

func copyUpdateFile(ctx context.Context, source *os.Root, name string, destination *os.Root, target, expected string, size int64, stamp os.FileInfo) (result updateCopy, err error) {
	if err := ctx.Err(); err != nil {
		return result, err
	}
	blocked, missing, err := updateParents(source, name)
	if err != nil {
		return result, err
	}
	if blocked || missing {
		return result, updateChanged()
	}
	in, err := fsutil.OpenRootRegular(source, name)
	if err != nil {
		return result, err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return result, err
	}
	if size < 0 || size > 4<<30 || before.Size() != size || stamp != nil && !sameUpdateFile(stamp, before) {
		return result, updateChanged()
	}
	if err := updateEnsureParents(destination, target); err != nil {
		return result, err
	}
	mode := os.FileMode(0o644)
	if stamp != nil {
		mode = stamp.Mode().Perm()
	}
	out, err := destination.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return result, err
	}
	defer func() {
		var statErr error
		result.info, statErr = out.Stat()
		err = errors.Join(err, statErr, out.Close())
	}()
	if stamp != nil {
		if err := out.Chmod(mode); err != nil {
			return result, err
		}
	}
	hash := sha1.New()
	buffer := archiveBuffers.Get().(*[]byte)
	result.size, err = io.CopyBuffer(struct{ io.Writer }{io.MultiWriter(out, hash)}, io.LimitReader(fsutil.ContextReader(ctx, in), size+1), *buffer)
	archiveBuffers.Put(buffer)
	result.hash = hex.EncodeToString(hash.Sum(nil))
	if err != nil {
		return result, err
	}
	after, err := in.Stat()
	if err != nil {
		return result, err
	}
	latest, err := source.Lstat(name)
	if err != nil {
		return result, err
	}
	if result.size != size || result.hash != expected || !sameUpdateFile(before, after) || !sameUpdateFile(before, latest) {
		return result, fmt.Errorf(locales.FileVerificationFailed(), path.Base(name))
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, out.Sync()
}

func publishUpdateFile(ctx context.Context, root *os.Root, source, target string, expected updateCopy) (updateCopy, error) {
	if err := ctx.Err(); err != nil {
		return updateCopy{}, err
	}
	if err := updateEnsureParents(root, target); err != nil {
		return updateCopy{}, err
	}
	info, err := root.Lstat(source)
	if err != nil {
		return updateCopy{}, err
	}
	if !sameUpdateFile(expected.info, info) {
		return updateCopy{}, updateChanged()
	}
	if err := root.Link(source, target); err == nil {
		return expected, nil
	} else if errors.Is(err, os.ErrExist) {
		return updateCopy{}, err
	}
	return copyUpdateFile(ctx, root, source, root, target, expected.hash, expected.size, expected.info)
}

func removeUpdateFile(root *os.Root, name string, expected os.FileInfo) error {
	if expected == nil {
		return nil
	}
	blocked, missing, err := updateParents(root, name)
	if err != nil {
		return err
	}
	if blocked {
		return updateChanged()
	}
	if missing {
		return nil
	}
	current, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !sameUpdateFile(expected, current) {
		return updateChanged()
	}
	return root.Remove(name)
}
