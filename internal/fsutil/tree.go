package fsutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type CopyProgress struct {
	Bytes, TotalBytes int64
	Files, TotalFiles int
}

type copyJob struct {
	root     *os.Root
	src, dst string
	info     os.FileInfo
}

type TreeCopier struct {
	ctx        context.Context
	cancel     context.CancelFunc
	dst        *os.Root
	permit     func(context.Context) (func(), error)
	progress   func(CopyProgress)
	jobs       chan copyJob
	workers    sync.WaitGroup
	finish     sync.Once
	mu, notify sync.Mutex
	state      CopyProgress
	entries    int
	last       time.Time
	err        error
	closeErr   error
}

func NewTreeCopier(ctx context.Context, dst *os.Root, permit func(context.Context) (func(), error), progress func(CopyProgress)) *TreeCopier {
	ctx, cancel := context.WithCancel(ctx)
	c := &TreeCopier{ctx: ctx, cancel: cancel, dst: dst, permit: permit, progress: progress, jobs: make(chan copyJob, 16)}
	for range 4 {
		c.workers.Go(func() {
			buffer := make([]byte, 64<<10)
			for job := range c.jobs {
				if err := c.copyFile(job, buffer); err != nil {
					c.fail(err)
					return
				}
			}
		})
	}
	return c
}

func (c *TreeCopier) Close() error {
	c.finish.Do(func() {
		close(c.jobs)
		c.workers.Wait()
		c.mu.Lock()
		c.closeErr = c.err
		c.mu.Unlock()
		if c.closeErr == nil {
			c.closeErr = c.ctx.Err()
		}
		c.report(true)
		c.cancel()
	})
	return c.closeErr
}

func (c *TreeCopier) Cancel() { c.cancel() }

func (c *TreeCopier) fail(err error) {
	c.mu.Lock()
	if c.err == nil {
		c.err = err
	}
	c.mu.Unlock()
	c.cancel()
}

func (c *TreeCopier) File(root *os.Root, src, dst string, required bool) (err error) {
	defer func() {
		if err != nil {
			c.fail(err)
		}
	}()
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if err := copyPath(src); err != nil {
		return err
	}
	if err := copyPath(dst); err != nil {
		return err
	}
	info, err := root.Lstat(src)
	if !required && errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf(locales.CannotCopyALinkOrSpecialFile(), src)
	}
	c.mu.Lock()
	c.entries++
	c.state.TotalFiles++
	over := c.entries > 200000 || info.Size() < 0 || info.Size() > 64<<30-c.state.TotalBytes
	if !over {
		c.state.TotalBytes += info.Size()
	}
	c.mu.Unlock()
	if over {
		return errors.New(locales.InstanceFileCountOrSizeExceedsTheCopyLimit())
	}
	select {
	case c.jobs <- copyJob{root: root, src: src, dst: dst, info: info}:
		return nil
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

func (c *TreeCopier) Tree(root *os.Root, include func(string, bool) bool) error {
	err := c.walk(root, ".", 0, include)
	if err != nil {
		c.fail(err)
	}
	return err
}

func (c *TreeCopier) walk(root *os.Root, path string, depth int, include func(string, bool) bool) error {
	if err := c.ctx.Err(); err != nil {
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
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			if err := c.ctx.Err(); err != nil {
				return err
			}
			name := filepath.Join(path, entry.Name())
			if include != nil && !include(name, entry.IsDir()) {
				continue
			}
			if err := copyPath(name); err != nil {
				return err
			}
			if entry.IsDir() {
				c.mu.Lock()
				c.entries++
				over := c.entries > 200000
				c.mu.Unlock()
				if over {
					return errors.New(locales.InstanceFileCountOrSizeExceedsTheCopyLimit())
				}
				if err := c.dst.MkdirAll(name, 0755); err != nil {
					return err
				}
				if err := c.walk(root, name, depth+1, include); err != nil {
					return err
				}
			} else if err := c.File(root, name, name, true); err != nil {
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

func copyPath(path string) error {
	if _, err := SafePath(".", filepath.ToSlash(path)); err != nil {
		return err
	}
	for part := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if err := ValidateID(part); err != nil {
			return err
		}
	}
	return nil
}

func (c *TreeCopier) copyFile(job copyJob, buffer []byte) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.permit != nil {
		release, err := c.permit(c.ctx)
		if err != nil {
			return err
		}
		defer release()
	}
	in, err := OpenRootRegular(job.root, job.src)
	if err != nil {
		return err
	}
	defer in.Close()
	before, err := in.Stat()
	if err != nil {
		return err
	}
	if !sameCopyFile(job.info, before) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	if err := c.dst.MkdirAll(filepath.Dir(job.dst), 0755); err != nil {
		return err
	}
	out, err := c.dst.OpenFile(job.dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	n, err := io.CopyBuffer(copyOutput{out, c}, io.LimitReader(contextReader{c.ctx, in}, before.Size()+1), buffer)
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
	current, err := job.root.Lstat(job.src)
	if err != nil {
		return err
	}
	if n != before.Size() || !sameCopyFile(before, after) || !sameCopyFile(before, current) {
		return errors.New(locales.SourceFilesChangedDuringCopyingPleaseTryAgain())
	}
	c.mu.Lock()
	c.state.Files++
	c.mu.Unlock()
	c.report(false)
	return c.ctx.Err()
}

func sameCopyFile(a, b os.FileInfo) bool {
	return b.Mode().IsRegular() && os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

type copyOutput struct {
	io.Writer
	copier *TreeCopier
}

func (w copyOutput) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.copier.mu.Lock()
	w.copier.state.Bytes += int64(n)
	w.copier.mu.Unlock()
	w.copier.report(false)
	return n, err
}

func (c *TreeCopier) report(force bool) {
	if c.progress == nil {
		return
	}
	c.notify.Lock()
	defer c.notify.Unlock()
	if !force && time.Since(c.last) < 100*time.Millisecond {
		return
	}
	c.last = time.Now()
	c.mu.Lock()
	state := c.state
	c.mu.Unlock()
	c.progress(state)
}
