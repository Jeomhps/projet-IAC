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

// EnableIfDisabled sets enabled=1 only if currently disabled; returns whether it changed.
func EnableIfDisabled(d *DB, id int) (bool, error) {
	res, err := d.Exec(`UPDATE machines SET enabled=1 WHERE id=? AND enabled=0`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// NeedReplacementRow represents a primary reservation that needs a replacement machine.
type NeedReplacementRow struct {
	Username       string     `db:"username"`
	PrimaryMID     int        `db:"primary_mid"`
	Until          *time.Time `db:"reserved_until"`
	HashedPassword *string    `db:"hashed_password"`
}

// LoadNeedsReplacement returns reservations whose primary machine is disabled and have no active replacement yet.
func LoadNeedsReplacement(d *DB) ([]NeedReplacementRow, error) {
	q := `
SELECT pr.username,
       pr.machine_id AS primary_mid,
       pr.reserved_until,
       pr.hashed_password
FROM reservations pr
JOIN machines pm ON pm.id = pr.machine_id
LEFT JOIN reservations rr
       ON rr.replacement_for_machine_id = pr.machine_id
      AND rr.username = pr.username
      AND (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
WHERE (pr.reserved_until IS NULL OR pr.reserved_until > UTC_TIMESTAMP())
  AND pr.replacement_for_machine_id IS NULL
  AND pm.enabled = 0
  AND rr.id IS NULL
ORDER BY pr.username ASC, pr.machine_id ASC`
	var rows []NeedReplacementRow
	if err := d.Select(&rows, q); err != nil {
		return nil, err
	}
	return rows, nil
}

// AllocateReplacement reserves a spare-pool machine for the user as a replacement for the given primary machine.
// It mirrors the user's reservation expiration and stores the hashed password for later provisioning.
// The operation is transactional and will roll back on any failure.
func AllocateReplacement(d *DB, username string, primaryMID int, until *time.Time, hashedPassword *string) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		if !ok {
			_ = tx.Rollback()
		}
	}()

	// Re-validate that a replacement is still required within the transaction.
	var still int
	err = tx.Get(&still, `
SELECT COUNT(1)
FROM reservations pr
JOIN machines pm ON pm.id = pr.machine_id
LEFT JOIN reservations rr
       ON rr.replacement_for_machine_id = pr.machine_id
      AND rr.username = pr.username
      AND (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
WHERE (pr.reserved_until IS NULL OR pr.reserved_until > UTC_TIMESTAMP())
  AND pr.replacement_for_machine_id IS NULL
  AND pm.enabled = 0
  AND pr.machine_id = ?
  AND pr.username = ?
  AND rr.id IS NULL
`, primaryMID, username)
	if err != nil || still == 0 {
		if err != nil {
			return err
		}
		_ = tx.Rollback()
		return nil
	}

	// Pick a spare machine: enabled=1, online=1, reserved=0, spare_pool=1
	var spareMID int
	err = tx.Get(&spareMID, `
SELECT id
FROM machines
WHERE spare_pool=1 AND enabled=1 AND online=1 AND reserved=0
ORDER BY reserve_fail_count ASC, id ASC
LIMIT 1 FOR UPDATE
`)
	if err != nil {
		return err
	}

	// Resolve user_id
	var uid int
	if err := tx.Get(&uid, `SELECT id FROM users WHERE username=?`, username); err != nil {
		return err
	}

	// Reserve the spare machine for the user with the same expiration.
	if until != nil {
		if _, err := tx.Exec(`
UPDATE machines
SET reserved=1, reserved_by=?, reserved_until=?
WHERE id=? AND reserved=0
`, username, until.UTC(), spareMID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`
UPDATE machines
SET reserved=1, reserved_by=?, reserved_until=NULL
WHERE id=? AND reserved=0
`, username, spareMID); err != nil {
			return err
		}
	}

	// Insert replacement reservation linked to the primary.
	if until != nil {
		if _, err := tx.Exec(`
INSERT INTO reservations (machine_id, user_id, username, reserved_until, hashed_password, replacement_for_machine_id)
VALUES (?,?,?,?,?,?)
`, spareMID, uid, username, until.UTC(), hashedPassword, primaryMID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`
INSERT INTO reservations (machine_id, user_id, username, reserved_until, hashed_password, replacement_for_machine_id)
VALUES (?,?,?,?,?,?)
`, spareMID, uid, username, nil, hashedPassword, primaryMID); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	ok = true
	return nil
}

// ReleaseReplacement removes a replacement reservation and frees the replacement machine.
func ReleaseReplacement(d *DB, replacementResID int, replacementMID int) error {
	tx, err := d.Beginx()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec(`DELETE FROM reservations WHERE id=?`, replacementResID); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE machines SET reserved=0, reserved_by=NULL, reserved_until=NULL WHERE id=?`, replacementMID); err != nil {
		return err
	}

	return tx.Commit()
}
