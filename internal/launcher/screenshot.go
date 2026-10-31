package launcher

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/UruhaLushia/tomori/internal/fsutil"
	"github.com/UruhaLushia/tomori/internal/instancecontent"
	"github.com/UruhaLushia/tomori/internal/minecraft"
	"github.com/UruhaLushia/tomori/internal/storage"
	"github.com/UruhaLushia/tomori/locales"
)

func (s *Service) ExportScreenshot(ctx context.Context, instance minecraft.Instance, source instancecontent.File, directory string) (_ string, err error) {
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	_, unlock, err := s.lockWorldRoots(ctx, nil, content)
	if err != nil {
		return "", err
	}
	defer unlock()
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	in, err := instancecontent.OpenScreenshot(content, source)
	if err != nil {
		return "", err
	}
	defer in.Close()
	space, err := storage.Assess(ctx, []storage.Requirement{{Directory: directory, Bytes: storage.Add(uint64(source.Size), uint64(source.Size))}})
	if err != nil {
		return "", err
	}
	if err := space.Check(); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	temp := ".tomori-screenshot-" + rand.Text()
	out, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer out.Close()
	info, err := out.Stat()
	if err != nil {
		return "", err
	}
	defer func() {
		if current, e := root.Lstat(temp); e == nil && os.SameFile(info, current) {
			err = errors.Join(err, root.Remove(temp))
		}
	}()
	n, err := io.CopyBuffer(out, io.LimitReader(fsutil.ContextReader(ctx, in), source.Size+1), make([]byte, 64<<10))
	if err != nil {
		return "", err
	}
	after, err := in.Stat()
	if err != nil {
		return "", err
	}
	if n != source.Size || !source.MatchesScreenshot(after) {
		return "", errors.New(locales.ScreenshotChangedRefreshAndTryAgain())
	}
	check, err := instancecontent.OpenScreenshot(content, source)
	if err != nil {
		return "", err
	}
	if err := check.Close(); err != nil {
		return "", err
	}
	if err := out.Sync(); err != nil {
		return "", err
	}
	info, err = out.Stat()
	if err != nil {
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	name := source.Name
	err = fsutil.PublishRootExclusive(ctx, root, temp, name, info)
	if errors.Is(err, os.ErrExist) {
		ext := filepath.Ext(name)
		name = strings.TrimSuffix(name, ext) + "-" + rand.Text()[:8] + ext
		err = fsutil.PublishRootExclusive(ctx, root, temp, name, info)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, name), nil
}

func (s *Service) TrashScreenshot(ctx context.Context, instance minecraft.Instance, source instancecontent.File, trash func(string) error) error {
	content := instance.ContentDirectory(s.cfg.GameForInstance(instance.Key()).Isolate)
	_, unlock, err := s.lockWorldRoots(ctx, nil, content)
	if err != nil {
		return err
	}
	defer unlock()
	release, err := s.client.FilePermit(ctx)
	if err != nil {
		return err
	}
	defer release()
	f, err := instancecontent.OpenScreenshot(content, source)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	current, err := os.Lstat(source.Path)
	if err != nil {
		return err
	}
	if !source.MatchesScreenshot(current) {
		return errors.New(locales.ScreenshotChangedRefreshAndTryAgain())
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return trash(source.Path)
}
