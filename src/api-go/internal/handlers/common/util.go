package common

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Package common provides small, shared helpers used across handlers.
// KISS: tiny functions, no shared mutable state, and clear, focused behavior.

// InventoryHost represents a single target line for an INI-style Ansible inventory.
type InventoryHost struct {
	Name     string
	Host     string
	Port     int
	User     string
	Password string
}

// WriteTempInventory writes a temporary one-off inventory file for the provided hosts.
// Returns the file path and a cleanup func that removes the file when called.
func WriteTempInventory(hosts []InventoryHost) (string, func(), error) {
	f, err := os.CreateTemp("", "inv-*.ini")
	if err != nil {
		return "", func() {}, err
	}
	var b strings.Builder
	for _, h := range hosts {
		// Keep a compact format; escape password for safety
		fmt.Fprintf(&b, "%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n",
			h.Name, h.Host, h.Port, h.User, SafeInvVal(h.Password))
	}
	if _, err := f.WriteString(b.String()); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", func() {}, err
	}
	_ = f.Close()
	cleanup := func() { _ = os.Remove(f.Name()) }
	return f.Name(), cleanup, nil
}

// BuildAnsibleArgs returns standard ansible-playbook args:
// - Verbosity based on LOG_LEVEL
// - Forks
// - Inventory path
// - Playbook path
// - Compact --extra-vars string
func BuildAnsibleArgs(level string, forks int, inventoryPath, playbook, extraVars string) []string {
	args := []string{"ansible-playbook"}
	if v := AnsibleVerbosityFlags(level); len(v) > 0 {
		args = append(args, v...)
	}
	args = append(args,
		"-f", strconv.Itoa(forks),
		"-i", inventoryPath,
		playbook,
		"--extra-vars", extraVars,
	)
	return args
}

// EnvForks parses ANSIBLE_FORKS from the environment with a default fallback.
func EnvForks(def int) int {
	if v := strings.TrimSpace(os.Getenv("ANSIBLE_FORKS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// SafeInvVal escapes characters that can break simple INI-like key=value formats.
func SafeInvVal(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ` `, `\ `)
	s = strings.ReplaceAll(s, `=`, `\=`)
	return s
}

var rePass = regexp.MustCompile(`(?m)(ansible_password=)(\S+)`)

// SanitizeInventory redacts ansible_password values for safe logging.
func SanitizeInventory(inv string) string {
	return rePass.ReplaceAllString(inv, "${1}***")
}

// LogLevel reads the LOG_LEVEL environment variable, defaulting to "info".
func LogLevel() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if v == "" {
		return "info"
	}
	return v
}

// StreamAnsible indicates whether to stream Ansible stdout/stderr directly.
// Useful for debug/trace modes; otherwise keep stderr only to report concise failures.
func StreamAnsible(level string) bool {
	return level == "debug" || strings.HasPrefix(level, "trace")
}

// AnsibleVerbosityFlags maps LOG_LEVEL to -v flags for ansible-playbook.
func AnsibleVerbosityFlags(level string) []string {
	switch level {
	case "trace3", "trace-3":
		return []string{"-vvv"}
	case "trace2", "trace-2":
		return []string{"-vv"}
	case "trace", "trace1", "trace-1":
		return []string{"-v"}
	default:
		return nil
	}
}

// HashSHA512Crypt uses openssl to generate a $6$-style SHA-512-crypt hash.
// This matches many Linux usermod defaults and avoids bringing extra Go deps.
func HashSHA512Crypt(password string) (string, error) {
	cmd := exec.Command("openssl", "passwd", "-6", password)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("openssl: %v: %s", err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// FormatTimePtr returns an RFC3339 time string or nil if the pointer is nil.
func FormatTimePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
