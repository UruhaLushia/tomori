package logstore

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type Store struct {
	mu     sync.Mutex
	dir    string
	limit  int64
	total  int64
	files  map[string]entry
	active map[string]*File
}

type entry struct {
	name string
	size int64
	at   time.Time
}

type File struct {
	store *Store
	file  *os.File
	name  string
	size  int64
}

func New(dir string, limit int64) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, limit: max(1, limit), active: make(map[string]*File)}
	if err := s.Prune(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) SetLimit(limit int64) {
	s.mu.Lock()
	s.limit = max(1, limit)
	s.mu.Unlock()
}

func (s *Store) Prune() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	s.files, s.total = make(map[string]entry), 0
	for _, item := range entries {
		if !item.Type().IsRegular() || !logName(item.Name()) {
			continue
		}
		info, err := item.Info()
		if err != nil {
			return err
		}
		s.record(item.Name(), info.Size(), info.ModTime())
	}
	return s.prune()
}

func (s *Store) Open(name string, appendFile bool) (*File, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".log") || s.active[name] != nil {
		return nil, fmt.Errorf(locales.InvalidLogFile(), name)
	}
	path := filepath.Join(s.dir, name)
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf(locales.LogPathIsNotARegularFile(), name)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if appendFile {
		flags = os.O_CREATE | os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(path, flags, 0o600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	w := &File{store: s, file: f, name: name, size: info.Size()}
	s.active[name] = w
	s.record(name, info.Size(), info.ModTime())
	return w, nil
}

func (f *File) Write(data []byte) (int, error) {
	s := f.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if f.file == nil {
		return 0, os.ErrClosed
	}
	limit := s.fileLimit()
	if int64(len(data)) <= limit && f.size > 0 && f.size+int64(len(data)) > limit {
		if err := f.rotate(); err != nil {
			return 0, err
		}
	}
	written := 0
	for len(data) > 0 {
		if f.size >= limit {
			if err := f.rotate(); err != nil {
				return written, err
			}
		}
		n := min(int64(len(data)), limit-f.size)
		count, err := f.file.Write(data[:n])
		f.size += int64(count)
		s.record(f.name, f.size, time.Now())
		written += count
		data = data[count:]
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

func (f *File) Close() error {
	f.store.mu.Lock()
	defer f.store.mu.Unlock()
	if f.store.active[f.name] == f {
		delete(f.store.active, f.name)
	}
	if f.file == nil {
		return nil
	}
	err := f.file.Close()
	f.file = nil
	return err
}

func (f *File) rotate() error {
	s := f.store
	if err := f.file.Close(); err != nil {
		return err
	}
	f.file = nil
	path, backup := filepath.Join(s.dir, f.name), f.name+".1"
	if err := s.remove(backup); err != nil {
		f.file, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		return err
	}
	if err := os.Rename(path, filepath.Join(s.dir, backup)); err != nil {
		f.file, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		return err
	}
	s.record(backup, f.size, time.Now())
	s.record(f.name, 0, time.Now())
	f.size = 0
	var err error
	f.file, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	return err
}

func (s *Store) record(name string, size int64, at time.Time) {
	s.total += size - s.files[name].size
	s.files[name] = entry{name, size, at}
}

func (s *Store) remove(name string) error {
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.total -= s.files[name].size
	delete(s.files, name)
	return nil
}

func (s *Store) prune() error {
	if s.total <= s.limit && len(s.files) <= 256 {
		return nil
	}
	target := s.limit * 3 / 4
	var candidates []entry
	for name, item := range s.files {
		if s.active[name] == nil {
			candidates = append(candidates, item)
		}
	}
	slices.SortFunc(candidates, func(a, b entry) int {
		if order := a.at.Compare(b.at); order != 0 {
			return order
		}
		return cmp.Compare(a.name, b.name)
	})
	for _, item := range candidates {
		if err := s.remove(item.name); err != nil {
			return err
		}
		if s.total <= target && len(s.files) <= 192 {
			return nil
		}
	}
	return nil
}

func (s *Store) fileLimit() int64 {
	return max(1, min(s.limit/int64(max(4, len(s.active)*2)), 16<<20))
}

func logName(name string) bool {
	return strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.1")
}
