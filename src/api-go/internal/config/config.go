package config

import (
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	// DB settings (from environment)
	DBHost     string
	DBPort     string
	DBName     string
	DBUser     string
	DBPassword string

	// Optional DB params (with sensible defaults)
	DBCharset      string // default: utf8mb4
	DBCollation    string // default: utf8mb4_unicode_ci
	DBTimeout      string // default: 5s
	DBReadTimeout  string // default: 5s
	DBWriteTimeout string // default: 5s

	// App settings
	JWTSecret           string
	AdminDefaultUser    string
	AdminDefaultPass    string
	AnsiblePlaybookPath string
}

func Load() Config {
	return Config{
		DBHost:     getenv("DB_HOST", "db"),
		DBPort:     getenv("DB_PORT", "3306"),
		DBName:     getenv("DB_NAME", "containers"),
		DBUser:     getenv("DB_USER", "appuser"),
		DBPassword: getenv("DB_PASSWORD", "apppass"),

		DBCharset:      getenv("DB_CHARSET", "utf8mb4"),
		DBCollation:    getenv("DB_COLLATION", "utf8mb4_unicode_ci"),
		DBTimeout:      getenv("DB_TIMEOUT", "5s"),
		DBReadTimeout:  getenv("DB_READ_TIMEOUT", "5s"),
		DBWriteTimeout: getenv("DB_WRITE_TIMEOUT", "5s"),

		JWTSecret:           getenv("JWT_SECRET", "change-me-in-prod"),
		AdminDefaultUser:    os.Getenv("ADMIN_DEFAULT_USERNAME"),
		AdminDefaultPass:    os.Getenv("ADMIN_DEFAULT_PASSWORD"),
		AnsiblePlaybookPath: getenv("ANSIBLE_PLAYBOOK", "/app/playbooks/create-users.yml"),
	}
}

// DSN builds a safe MySQL DSN using driver config, including timeouts and charset.
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

	// For DB TLS, you can register a tls.Config and set cfg.TLSConfig to the name.
	// Example: mysql.RegisterTLSConfig("custom", tlsCfg); cfg.TLSConfig = "custom"

	return cfg.FormatDSN()
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
