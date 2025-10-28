package db

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type DB struct{ *sqlx.DB }

func OpenFromDATABASE_URL(databaseURL string) (*DB, error) {
	dsn, err := convertToMySQLDSN(databaseURL)
	if err != nil { return nil, err }
	db, err := sqlx.Open("mysql", dsn)
	if err != nil { return nil, err }
	db.SetConnMaxLifetime(2 * time.Hour)
	db.SetMaxIdleConns(10)
	db.SetMaxOpenConns(50)
	// Wait loop
	for i := 0; i < 120; i++ {
		if err := db.Ping(); err == nil { return &DB{db}, nil }
		time.Sleep(time.Second)
	}
	return nil, fmt.Errorf("database not reachable")
}

func convertToMySQLDSN(u string) (string, error) {
	u = strings.Replace(u, "mysql+pymysql://", "mysql://", 1)
	pu, err := url.Parse(u)
	if err != nil { return "", err }
	user := pu.User.Username()
	pass, _ := pu.User.Password()
	host := pu.Host
	db := strings.TrimPrefix(pu.Path, "/")
	q := pu.RawQuery
	if !regexp.MustCompile(`(^|&)parseTime=`).MatchString(q) {
		if q == "" { q = "parseTime=true" } else { q += "&parseTime=true" }
	}
	if !regexp.MustCompile(`(^|&)charset=`).MatchString(q) {
		if q == "" { q = "charset=utf8mb4" } else { q += "&charset=utf8mb4" }
	}
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", user, pass, host, db, q), nil
}

func EnsureSchema(db *DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INT AUTO_INCREMENT PRIMARY KEY,
			username VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			is_admin BOOLEAN NOT NULL DEFAULT 0,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS machines (
			id INT AUTO_INCREMENT PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			host VARCHAR(255) NOT NULL,
			port INT NOT NULL DEFAULT 22,
			user VARCHAR(255) NOT NULL,
			password VARCHAR(255) NOT NULL,
			reserved BOOLEAN NOT NULL DEFAULT 0,
			reserved_by VARCHAR(255) NULL,
			reserved_until DATETIME NULL,
			enabled BOOLEAN NOT NULL DEFAULT 1,
			online BOOLEAN NOT NULL DEFAULT 1,
			last_seen_at DATETIME NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
		`CREATE TABLE IF NOT EXISTS reservations (
			id INT AUTO_INCREMENT PRIMARY KEY,
			machine_id INT NOT NULL,
			user_id INT NOT NULL,
			username VARCHAR(255) NOT NULL,
			reserved_until DATETIME NULL,
			CONSTRAINT fk_res_machine FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE,
			CONSTRAINT fk_res_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil { return err }
	}
	return nil
}

func EnsureDefaultAdmin(db *DB, u, p string) error {
	var count int
	if err := db.Get(&count, "SELECT COUNT(1) FROM users WHERE username=?", u); err != nil {
		return err
	}
	if count > 0 { return nil }
	hash, _ := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	_, err := db.Exec("INSERT INTO users (username,password_hash,is_admin) VALUES (?,?,1)", u, string(hash))
	return err
}

// Models
type User struct {
	ID           int       `db:"id"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	IsAdmin      bool      `db:"is_admin"`
	CreatedAt    time.Time `db:"created_at"`
}

type Machine struct {
	ID            int        `db:"id"`
	Name          string     `db:"name"`
	Host          string     `db:"host"`
	Port          int        `db:"port"`
	User          string     `db:"user"`
	Password      string     `db:"password"`
	Reserved      bool       `db:"reserved"`
	ReservedBy    *string    `db:"reserved_by"`
	ReservedUntil *time.Time `db:"reserved_until"`
	Enabled       bool       `db:"enabled"`
	Online        bool       `db:"online"`
	LastSeenAt    *time.Time `db:"last_seen_at"`
}

type Reservation struct {
	ID            int        `db:"id"`
	MachineID     int        `db:"machine_id"`
	UserID        int        `db:"user_id"`
	Username      string     `db:"username"`
	ReservedUntil *time.Time `db:"reserved_until"`
}
