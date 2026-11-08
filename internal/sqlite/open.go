package sqlite

import (
	"database/sql"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type Client struct {
	db           *gorm.DB
	conn         *sql.DB
	writable     bool
	mu           sync.Mutex
	holds        int
	closing      bool
	closed       bool
	writes       chan pendingWrite
	stopWrites   chan struct{}
	writerDone   chan struct{}
	writerClosed bool
}

func Open(path string, write bool, models ...any) (*Client, error) {
	u := url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(path), "/")}
	mode := "rw"
	if write {
		mode = "rwc"
	}
	query := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(3000)"}}
	if write {
		query.Add("_pragma", "journal_mode(WAL)")
		query.Add("_pragma", "synchronous(NORMAL)")
	}
	u.RawQuery = query.Encode()
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, err
	}
	conn, err := db.DB()
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	client := &Client{
		db:           db,
		conn:         conn,
		writable:     write,
		stopWrites:   make(chan struct{}),
		writerDone:   make(chan struct{}),
		writerClosed: !write,
	}
	if write && len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	if write {
		client.writes = make(chan pendingWrite, 64)
		go client.writeLoop()
	} else {
		close(client.writerDone)
	}
	return client, nil
}
