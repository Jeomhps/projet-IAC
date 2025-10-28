package lock

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type Advisory struct {
	conn     *sql.Conn
	lockName string
	acquired bool
}

func Acquire(db *sql.DB, name string, timeoutSeconds int) (*Advisory, error) {
	c, err := db.Conn(context.Background())
	if err != nil {
		return nil, err
	}

	a := &Advisory{conn: c, lockName: name}
	var got sql.NullInt64

	ctx := context.Background()
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second+500*time.Millisecond)
		defer cancel()
	}

	if err := c.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, timeoutSeconds).Scan(&got); err != nil {
		_ = c.Close()
		return nil, err
	}
	if !got.Valid || got.Int64 != 1 {
		_ = c.Close()
		return nil, fmt.Errorf("could not acquire MySQL advisory lock %q (result=%v)", name, got)
	}
	a.acquired = true
	return a, nil
}

func (a *Advisory) Release() {
	if a == nil || a.conn == nil {
		return
	}
	if a.acquired {
		_, _ = a.conn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", a.lockName)
	}
	_ = a.conn.Close()
}
