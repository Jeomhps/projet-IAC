package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	*sqlx.DB
}

func Open(dsn string) (*DB, error) {
	xdb, err := sqlx.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := xdb.Ping(); err != nil {
		return nil, err
	}
	d := &DB{DB: xdb}
	// dev-only: ensure schema inline
	if err := d.ensureSchema(context.Background()); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *DB) Close() error { return d.DB.Close() }

// Exported helpers expected by main.go

func EnsureSchema(d *DB) error {
	return d.ensureSchema(context.Background())
}

func EnsureDefaultAdmin(d *DB, username, password string) error {
	if username == "" || password == "" {
		return nil
	}
	var count int
	if err := d.Get(&count, "SELECT COUNT(*) FROM users WHERE username=?", username); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if count == 0 {
		_, err = d.Exec("INSERT INTO users (username, password_hash, is_admin) VALUES (?,?,1)", username, string(hash))
		return err
	}
	_, err = d.Exec("UPDATE users SET password_hash=?, is_admin=1 WHERE username=?", string(hash), username)
	return err
}

// Domain models (must match handlers)

type User struct {
	ID           int64     `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	IsAdmin      bool      `db:"is_admin"`
	CreatedAt    time.Time `db:"created_at"`
}

type Machine struct {
	ID            int64          `db:"id"`
	Name          string         `db:"name"`
	Host          string         `db:"host"`
	Port          int            `db:"port"`
	User          string         `db:"user"`
	Password      sql.NullString `db:"password"` // nullable after enrollment
	Enabled       bool           `db:"enabled"`
	Online        bool           `db:"online"`
	Reserved      bool           `db:"reserved"`
	ReservedBy    *string        `db:"reserved_by"`
	ReservedUntil *time.Time     `db:"reserved_until"`
	LastSeenAt    *time.Time     `db:"last_seen_at"`
	AuthType      string         `db:"auth_type"` // "password" | "ssh_key"
	CreatedAt     time.Time      `db:"created_at"`
}

type Reservation struct {
	ID            int64      `db:"id"`
	UserID        int64      `db:"user_id"`
	Username      string     `db:"username"`
	MachineID     int64      `db:"machine_id"`
	ReservedUntil *time.Time `db:"reserved_until"`
	CreatedAt     time.Time  `db:"created_at"`
	// optional
	ExpiresAt           *time.Time `db:"expires_at"`
	Count               *int       `db:"count"`
	DurationMinutes     *int       `db:"duration_minutes"`
	ReservationPassword *string    `db:"reservation_password"`
}

// Convenience wrappers (used by handlers)

func (d *DB) Select(dest interface{}, query string, args ...any) error { return d.DB.Select(dest, query, args...) }
func (d *DB) Get(dest interface{}, query string, args ...any) error    { return d.DB.Get(dest, query, args...) }
func (d *DB) MustBegin() *sqlx.Tx                                      { return d.DB.MustBegin() }

// Dev-time schema (inline DDL)

func (d *DB) ensureSchema(ctx context.Context) error {
	stmts := []string{
		// users
		`CREATE TABLE IF NOT EXISTS users (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			is_admin TINYINT(1) NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		// machines — only change: auth_type; new machines start disabled
		`CREATE TABLE IF NOT EXISTS machines (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			host VARCHAR(255) NOT NULL,
			port INT NOT NULL,
			user VARCHAR(255) NOT NULL DEFAULT 'root',
			password TEXT NULL,

			enabled TINYINT(1) NOT NULL DEFAULT 0,
			online TINYINT(1) NOT NULL DEFAULT 1,
			reserved TINYINT(1) NOT NULL DEFAULT 0,
			reserved_by VARCHAR(255) NULL,
			reserved_until DATETIME NULL,
			last_seen_at DATETIME NULL,

			auth_type VARCHAR(16) NOT NULL DEFAULT 'password',

			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,

		// reservations (unchanged)
		`CREATE TABLE IF NOT EXISTS reservations (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			machine_id BIGINT NOT NULL,
			user_id BIGINT NOT NULL,
			username VARCHAR(255) NOT NULL,
			reserved_until DATETIME NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

			expires_at DATETIME NULL,
			count INT NULL,
			duration_minutes INT NULL,
			reservation_password VARCHAR(255) NULL,

			INDEX (machine_id),
			INDEX (user_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`,
	}

	for _, s := range stmts {
		if _, err := d.DB.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}
