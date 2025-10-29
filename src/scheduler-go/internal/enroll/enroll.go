package enroll

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Machine struct {
	ID       int64
	Name     string
	Host     string
	Port     int
	User     string
	Password string
	AuthType string
	Enabled  bool
}

type Service struct {
	DB                *sql.DB
	PlaybookPath      string // /app/playbooks/enroll-ssh.yml
	ControllerKeyPath string // /app/secrets/ssh/scheduler_ed25519
	TargetUser        string // iac
	Forks             int
	TempDir           string
}

func (s Service) Run(ctx context.Context) (int, error) {
	machines, err := s.findTargets(ctx)
	if err != nil {
		return 0, err
	}
	if len(machines) == 0 {
		return 0, nil
	}
	ok := 0
	for _, m := range machines {
		inv, err := s.writeInventory(m)
		if err != nil {
			return ok, fmt.Errorf("inventory %s: %w", m.Name, err)
		}
		defer os.Remove(inv)

		args := []string{
			"-i", inv, s.PlaybookPath,
			"--forks", fmt.Sprintf("%d", s.Forks),
			"--extra-vars", fmt.Sprintf("target_user=%s local_key_dir=%s local_private_key_file=%s",
				s.TargetUser, filepath.Dir(s.ControllerKeyPath), s.ControllerKeyPath),
		}
		cmd := exec.CommandContext(ctx, "ansible-playbook", args...)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			// continue with others
			continue
		}
		if err := s.flipToKey(ctx, m.ID); err != nil {
			return ok, fmt.Errorf("flip %s: %w", m.Name, err)
		}
		ok++
	}
	return ok, nil
}

func (s Service) findTargets(ctx context.Context) ([]Machine, error) {
	const q = `
SELECT id, name, host, port, user, password, auth_type, enabled
FROM machines
WHERE enabled = 0 AND auth_type = 'password'
`
	rows, err := s.DB.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Machine
	for rows.Next() {
		var m Machine
		if err := rows.Scan(&m.ID, &m.Name, &m.Host, &m.Port, &m.User, &m.Password, &m.AuthType, &m.Enabled); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s Service) writeInventory(m Machine) (string, error) {
	dir := s.TempDir
	if dir == "" {
		dir = os.TempDir()
	}
	var b strings.Builder
	b.WriteString("[targets]\n")
	sshUser := m.User
	if strings.TrimSpace(sshUser) == "" {
		sshUser = "root"
	}
	fmt.Fprintf(&b, "%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n",
		m.Name, m.Host, m.Port, sshUser, escapeInv(m.Password))

	path := filepath.Join(dir, fmt.Sprintf("inv_enroll_%s_%d.ini", m.Name, time.Now().UnixNano()))
	return path, os.WriteFile(path, []byte(b.String()), 0600)
}

func (s Service) flipToKey(ctx context.Context, id int64) error {
	const up = `
UPDATE machines
   SET auth_type='ssh_key',
       enabled=1,
       password=NULL
 WHERE id=?`
	_, err := s.DB.ExecContext(ctx, up, id)
	return err
}

func escapeInv(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ` `, `\ `)
	s = strings.ReplaceAll(s, `=`, `\=`)
	return s
}
