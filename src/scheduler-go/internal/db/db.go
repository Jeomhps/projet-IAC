package db

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
)

type DB struct{ *sqlx.DB }

func Open(dsn string) (*DB, error) {
	db, err := sqlx.Open("mysql", dsn)
	if err != nil { return nil, err }
	db.SetConnMaxLifetime(2 * time.Hour)
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(50)

	for i := 0; i < 120; i++ {
		if err := db.Ping(); err == nil {
			return &DB{db}, nil
		}
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("database not reachable")
}

type ExpiredRow struct {
	ResID   int    `db:"res_id"`
	MID     int    `db:"machine_id"`
	User    string `db:"username"`
	Name    string `db:"name"`
	Host    string `db:"host"`
	Port    int    `db:"port"`
	SSHUser string `db:"user"`
	Pass    string `db:"password"`
}

func LoadExpired(d *DB) ([]ExpiredRow, error) {
	q := `
SELECT r.id AS res_id, r.machine_id, r.username,
       m.name, m.host, m.port, m.user, m.password
FROM reservations r
JOIN machines m ON m.id = r.machine_id
WHERE r.reserved_until IS NOT NULL
  AND r.reserved_until <= UTC_TIMESTAMP()
ORDER BY r.username ASC, r.id ASC`
	var rows []ExpiredRow
	if err := d.Select(&rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

func ClearReservationsAndRelease(d *DB, pairs [][2]int) error {
	tx, err := d.Beginx()
	if err != nil { return err }
	defer func() {
		if err != nil { _ = tx.Rollback() }
	}()
	for _, p := range pairs {
		resID := p[0]
		mID := p[1]
		if _, err = tx.Exec("UPDATE machines SET reserved=0, reserved_by=NULL, reserved_until=NULL WHERE id=?", mID); err != nil {
			return err
		}
		if _, err = tx.Exec("DELETE FROM reservations WHERE id=?", resID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
