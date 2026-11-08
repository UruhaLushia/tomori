package cache

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type entry struct {
	Key       string    `gorm:"primaryKey"`
	Value     []byte    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"index"`
}

type Store struct {
	db      *gorm.DB
	mu      sync.Mutex
	holds   int
	closing bool
	closed  bool
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "cache.db")
	dsn := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}
	query := url.Values{}
	query.Add("_pragma", "busy_timeout(3000)")
	query.Add("_pragma", "journal_mode(DELETE)")
	dsn.RawQuery = query.Encode()
	db, err := gorm.Open(sqlite.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&entry{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.db == nil {
		return nil
	}
	if s.holds > 0 {
		s.closing = true
		return nil
	}
	conn, err := s.db.DB()
	if err != nil {
		return err
	}
	s.closed = true
	return conn.Close()
}

func (s *Store) hold() (*gorm.DB, func(), bool) {
	if s == nil {
		return nil, nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.db == nil {
		return nil, nil, false
	}
	s.holds++
	var once sync.Once
	release := func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.holds--
			if s.holds == 0 && s.closing && !s.closed {
				if conn, err := s.db.DB(); err == nil {
					_ = conn.Close()
				}
				s.closed = true
			}
		})
	}
	return s.db, release, true
}

func (s *Store) Hold() func() {
	_, release, ok := s.hold()
	if !ok {
		return nil
	}
	return release
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, time.Time, bool, error) {
	if key == "" {
		return nil, time.Time{}, false, nil
	}
	db, release, ok := s.hold()
	if !ok {
		return nil, time.Time{}, false, nil
	}
	defer release()
	var value entry
	err := db.WithContext(ctx).Where(&entry{Key: key}).Limit(1).Find(&value).Error
	if err != nil {
		return nil, time.Time{}, false, err
	}
	if value.Key == "" {
		return nil, time.Time{}, false, nil
	}
	return value.Value, value.UpdatedAt, true, nil
}

func (s *Store) Put(ctx context.Context, key string, value []byte) error {
	if key == "" || len(value) == 0 {
		return nil
	}
	db, release, ok := s.hold()
	if !ok {
		return nil
	}
	defer release()
	return db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&entry{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}
