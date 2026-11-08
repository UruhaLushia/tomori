package cache

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/UruhaLushia/tomori/internal/sqlite"
)

type entry struct {
	Key       string    `gorm:"primaryKey"`
	Value     []byte    `gorm:"not null"`
	UpdatedAt time.Time `gorm:"index"`
}

type Store struct {
	db *sqlite.Client
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := sqlite.Open(filepath.Join(dir, "cache.db"), true, &entry{})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Hold() func() {
	if s == nil {
		return nil
	}
	return s.db.Hold()
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, time.Time, bool, error) {
	if key == "" {
		return nil, time.Time{}, false, nil
	}
	var value entry
	if err := s.db.Read(ctx, &value, "key = ?", key); err != nil {
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
	return s.db.Write(ctx, key, entry{Key: key, Value: value, UpdatedAt: time.Now()})
}
