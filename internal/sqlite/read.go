package sqlite

import (
	"context"
	"errors"
	"sync"

	"gorm.io/gorm"
)

func (c *Client) Read(ctx context.Context, result any, query string, args ...any) error {
	if err := ctx.Err(); err != nil {
		return nil
	}
	_, release, ok := c.hold()
	if !ok {
		return nil
	}
	defer release()
	db := c.db.WithContext(ctx)
	if query != "" {
		db = db.Where(query, args...)
	}
	err := db.Find(result).Error
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (c *Client) Hold() func() {
	_, release, ok := c.hold()
	if !ok {
		return nil
	}
	return release
}

func (c *Client) hold() (*gorm.DB, func(), bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.db == nil {
		return nil, nil, false
	}
	c.holds++
	var once sync.Once
	release := func() {
		once.Do(func() {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.holds--
			if c.holds == 0 && c.closing && c.writerClosed && !c.closed {
				c.closed = true
				_ = c.conn.Close()
			}
		})
	}
	return c.db, release, true
}
