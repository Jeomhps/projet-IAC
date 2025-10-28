package config

import (
	"os"
)

type Config struct {
	DatabaseURL         string
	JWTSecret           string
	AdminDefaultUser    string
	AdminDefaultPass    string
	AnsiblePlaybookPath string
}

func Load() Config {
	// Default to playbooks copied inside the container at /app/playbooks
	defaultPlaybook := "/app/playbooks/create-users.yml"

	return Config{
		DatabaseURL:         getenv("DATABASE_URL", "mysql://appuser:apppass@tcp(db:3306)/containers?parseTime=true&charset=utf8mb4"),
		JWTSecret:           getenv("JWT_SECRET", "change-me-in-prod"),
		AdminDefaultUser:    os.Getenv("ADMIN_DEFAULT_USERNAME"),
		AdminDefaultPass:    os.Getenv("ADMIN_DEFAULT_PASSWORD"),
		AnsiblePlaybookPath: getenv("ANSIBLE_PLAYBOOK", defaultPlaybook),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" { return v }
	return def
}
