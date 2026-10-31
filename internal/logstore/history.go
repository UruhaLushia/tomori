package logstore

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/internal/fsutil"
)

type HistoryFile struct {
	Name     string
	Size     int64
	Modified time.Time
}

func (s *Store) History(ctx context.Context) ([]HistoryFile, error) {
	f, err := os.Open(s.dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var files []HistoryFile
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := f.ReadDir(64)
		for _, item := range entries {
			if !item.Type().IsRegular() || !logName(item.Name()) {
				continue
			}
			info, err := item.Info()
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			files = append(files, HistoryFile{item.Name(), info.Size(), info.ModTime()})
		}
		if len(files) > 512 {
			sortHistory(files)
			files = files[:256]
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	sortHistory(files)
	return files[:min(256, len(files))], nil
}

func sortHistory(files []HistoryFile) {
	slices.SortFunc(files, func(a, b HistoryFile) int {
		if order := b.Modified.Compare(a.Modified); order != 0 {
			return order
		}
		return strings.Compare(a.Name, b.Name)
	})
}

func (s *Store) Tail(ctx context.Context, name string) ([]string, bool, error) {
	if filepath.Base(name) != name || !logName(name) {
		return nil, false, os.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, false, err
	}
	defer root.Close()
	f, err := fsutil.OpenRootGrowingFile(root, name)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	const limit = 2 << 20
	start := max(0, info.Size()-limit)
	data := make([]byte, min(info.Size(), limit))
	n, err := f.ReadAt(data, start)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, false, err
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	text := string(data[:n])
	truncated := start > 0
	if start > 0 {
		_, text, _ = strings.Cut(text, "\n")
	}
	text = strings.TrimSuffix(text, "\n")
	end, count := len(text), 0
	for i := end - 1; i >= 0; i-- {
		if text[i] == '\n' {
			count++
			if count == 2000 {
				text, truncated = text[i+1:], true
				break
			}
		}
	}
	if text == "" {
		return nil, truncated, nil
	}
	return strings.Split(text, "\n"), truncated, nil
}
