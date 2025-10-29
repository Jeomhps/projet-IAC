package db

import (
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

type DB struct{ *sqlx.DB }

func Open(dsn string) (*DB, error) {
	db, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
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
  AND m.enabled=1
ORDER BY r.username ASC, r.id ASC`
	var rows []ExpiredRow
	if err := d.Select(&rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

func ClearReservationsAndRelease(d *DB, pairs [][2]int) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
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

// MachineRow is a minimal view of machines needed for SSH health checks.
type MachineRow struct {
	ID      int    `db:"id"`
	Name    string `db:"name"`
	Host    string `db:"host"`
	Port    int    `db:"port"`
	SSHUser string `db:"user"`
	Pass    string `db:"password"`
	Enabled bool   `db:"enabled"`
}

// LoadMachines returns all machines with SSH connection details.
func LoadMachines(d *DB) ([]MachineRow, error) {
	const q = `SELECT id, name, host, port, user, password, enabled FROM machines ORDER BY id ASC`
	var rows []MachineRow
	if err := d.Select(&rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// TouchMachineLastSeen updates last_seen_at to current UTC timestamp.
func TouchMachineLastSeen(d *DB, id int) error {
	_, err := d.Exec(`UPDATE machines SET last_seen_at=UTC_TIMESTAMP() WHERE id=?`, id)
	return err
}

// SetMachineEnabled sets the enabled flag for a machine.
func SetMachineEnabled(d *DB, id int, enabled bool) error {
	_, err := d.Exec(`UPDATE machines SET enabled=? WHERE id=?`, enabled, id)
	return err
}

// LoadExpiredForMachine returns expired reservations for a specific machine.
// It mirrors LoadExpired but filters to a single machine_id.
func LoadExpiredForMachine(d *DB, machineID int) ([]ExpiredRow, error) {
	q := `
SELECT r.id AS res_id, r.machine_id, r.username,
       m.name, m.host, m.port, m.user, m.password
FROM reservations r
JOIN machines m ON m.id = r.machine_id
WHERE r.reserved_until IS NOT NULL
  AND r.reserved_until <= UTC_TIMESTAMP()
  AND m.enabled=1
  AND r.machine_id = ?
ORDER BY r.username ASC, r.id ASC`
	var rows []ExpiredRow
	if err := d.Select(&rows, q, machineID); err != nil {
		return nil, err
	}
	return rows, nil
}
