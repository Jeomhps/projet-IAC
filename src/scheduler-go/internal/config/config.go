package config

import (
	"os"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	DBHost        string
	DBPort        string
	DBName        string
	DBUser        string
	DBPassword    string
	DBCharset     string
	DBCollation   string
	DBTimeout     string
	DBReadTimeout string
	DBWriteTimeout string

	IntervalSec int
	Once        bool
	LockName    string
	LockTimeout int

	PlaybookPath string
	Forks        int
	TempDir      string
}

func Load() Config {
	return Config{
		DBHost:         getenv("DB_HOST", "db"),
		DBPort:         getenv("DB_PORT", "3306"),
		DBName:         getenv("DB_NAME", "containers"),
		DBUser:         getenv("DB_USER", "appuser"),
		DBPassword:     getenv("DB_PASSWORD", "apppass"),
		DBCharset:      getenv("DB_CHARSET", "utf8mb4"),
		DBCollation:    getenv("DB_COLLATION", "utf8mb4_unicode_ci"),
		DBTimeout:      getenv("DB_TIMEOUT", "5s"),
		DBReadTimeout:  getenv("DB_READ_TIMEOUT", "5s"),
		DBWriteTimeout: getenv("DB_WRITE_TIMEOUT", "5s"),

		IntervalSec: atoi(getenv("SCHEDULER_INTERVAL", "60"), 60),
		Once:        false,
		LockName:    getenv("SCHEDULER_LOCK_NAME", "reservation-expiry-cleanup"),
		LockTimeout: atoi(getenv("DB_LOCK_TIMEOUT", "0"), 0),

		PlaybookPath: getenv("ANSIBLE_PLAYBOOK", "/app/playbooks/create-users.yml"),
		Forks:        atoi(getenv("ANSIBLE_FORKS", "5"), 5),
		TempDir:      defaultTmpDir(),
	}
}

func (c Config) DSN() string {
	cfg := mysql.NewConfig()
	cfg.User = c.DBUser
	cfg.Passwd = c.DBPassword
	cfg.Net = "tcp"
	cfg.Addr = c.DBHost + ":" + c.DBPort
	cfg.DBName = c.DBName
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = c.DBCharset
	cfg.Params["collation"] = c.DBCollation
	cfg.Params["timeout"] = c.DBTimeout
	cfg.Params["readTimeout"] = c.DBReadTimeout
	cfg.Params["writeTimeout"] = c.DBWriteTimeout
	return cfg.FormatDSN()
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func atoi(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func defaultTmpDir() string {
	if st, err := os.Stat("/dev/shm"); err == nil && st.IsDir() {
		return "/dev/shm"
	}
	return os.TempDir()
}
