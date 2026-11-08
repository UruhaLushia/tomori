package sqlite

import (
	"context"
	"errors"
	"reflect"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type pendingWrite struct {
	key   string
	value any
}

func (c *Client) Write(ctx context.Context, key string, value any) error {
	if !c.writable || key == "" {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil
	}
	c.mu.Lock()
	if c.closed || c.closing {
		c.mu.Unlock()
		return nil
	}
	writes, stop := c.writes, c.stopWrites
	c.mu.Unlock()
	select {
	case writes <- pendingWrite{key: key, value: value}:
	case <-stop:
	case <-ctx.Done():
	}
	return nil
}

func (c *Client) Replace(ctx context.Context, previous, values any) error {
	if !c.writable {
		return nil
	}
	_, release, ok := c.hold()
	if !ok {
		return nil
	}
	defer release()
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Find(previous).Error; err != nil {
			return err
		}
		rows := reflect.ValueOf(previous)
		if rows.Kind() == reflect.Pointer && rows.Elem().Kind() == reflect.Slice && rows.Elem().Len() > 0 {
			if err := tx.Delete(previous).Error; err != nil {
				return err
			}
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(values).Error
	})
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed || c.conn == nil {
		c.mu.Unlock()
		return nil
	}
	if !c.closing {
		c.closing = true
		close(c.stopWrites)
	}
	if c.holds > 0 {
		c.mu.Unlock()
		return nil
	}
	writerDone, writerClosed := c.writerDone, c.writerClosed
	c.mu.Unlock()
	if !writerClosed {
		<-writerDone
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.holds > 0 || c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

func (c *Client) writeLoop() {
	defer close(c.writerDone)
	batch := make(map[string]pendingWrite)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		groups := make(map[reflect.Type][]reflect.Value)
		for _, write := range batch {
			value := reflect.ValueOf(write.value)
			if value.Kind() == reflect.Pointer {
				value = value.Elem()
			}
			if value.IsValid() && value.Kind() == reflect.Struct {
				groups[value.Type()] = append(groups[value.Type()], value)
			}
		}
		for typ, values := range groups {
			rows := reflect.MakeSlice(reflect.SliceOf(typ), 0, len(values))
			for _, value := range values {
				rows = reflect.Append(rows, value)
			}
			_ = c.db.WithContext(context.Background()).Clauses(clause.OnConflict{UpdateAll: true}).CreateInBatches(rows.Interface(), 64).Error
		}
		clear(batch)
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case write := <-c.writes:
			batch[write.key] = write
			if len(batch) >= 32 {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-c.stopWrites:
			for {
				select {
				case write := <-c.writes:
					batch[write.key] = write
				default:
					flush()
					c.mu.Lock()
					c.writerClosed = true
					c.mu.Unlock()
					return
				}
			}
		}
	}
}
