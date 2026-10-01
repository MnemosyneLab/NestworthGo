package sqlite

import (
	"context"
	"database/sql/driver"
	"errors"
)

// DrainForReplication reserves the sole pool slot, waits for outstanding rows
// and transactions, and closes the underlying driver connection before fn.
// Queued queries cannot reopen a connection until fn finishes. The sql.DB and
// repository pointers stay stable; ErrBadConn makes the pool replace this slot.
// This deliberately relies on the pinned modernc driver's idempotent Close.
// Litestream must not close POSIX file descriptors while app connections exist.
func (db *DB) DrainForReplication(ctx context.Context, fn func() error) error {
	if db == nil || db.SQL == nil {
		return fn()
	}
	conn, err := db.SQL.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var result error
	err = conn.Raw(func(raw any) error {
		if closeErr := raw.(driver.Conn).Close(); closeErr != nil {
			return closeErr
		}
		result = fn()
		return driver.ErrBadConn
	})
	if err != nil && !errors.Is(err, driver.ErrBadConn) {
		return err
	}
	return result
}
