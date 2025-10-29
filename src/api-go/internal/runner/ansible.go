package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PlaybookRunner is a small helper to run Ansible playbooks with consistent
// logging and verbosity behavior. It mirrors the scheduler's runner so both
// components use the same engine/config under the hood.
type PlaybookRunner struct {
	Playbook string
	Forks    int
}

// RunDeleteUser executes the delete-user action for a set of hosts provided via
// an INI-style inventory path. It streams stdout/stderr so logs appear in the
// process logs (consistent with other parts of the system).
func (r PlaybookRunner) RunDeleteUser(ctx context.Context, inventoryPath, username string) error {
	args := []string{"ansible-playbook"}
	if v := verbosityFlags(); len(v) > 0 {
		args = append(args, v...)
	}
	args = append(args,
		"-f", fmt.Sprintf("%d", r.Forks),
		"-i", inventoryPath,
		r.Playbook,
		"--extra-vars", fmt.Sprintf("username=%s user_action=delete ansible_ssh_timeout=15", username),
	)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RunDeleteUserSingleHost writes a temporary one-host inventory and runs the
// delete-user action against that single host, streaming logs as usual.
func (r PlaybookRunner) RunDeleteUserSingleHost(ctx context.Context, machineName, host string, port int, sshUser, password, username string) error {
	f, err := os.CreateTemp("", "inv-*.ini")
	if err != nil {
		return err
	}
	// Write inventory entry
	line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n", machineName, host, port, sshUser, password)
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	// Ensure data is flushed before invoking Ansible
	_ = f.Close()
	defer os.Remove(f.Name())

	// Build args and invoke ansible-playbook
	args := []string{"ansible-playbook"}
	if v := verbosityFlags(); len(v) > 0 {
		args = append(args, v...)
	}
	args = append(args,
		"-f", fmt.Sprintf("%d", r.Forks),
		"-i", f.Name(),
		r.Playbook,
		"--extra-vars", fmt.Sprintf("username=%s user_action=delete ansible_ssh_timeout=15", username),
	)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// verbosityFlags maps LOG_LEVEL to Ansible -v flags for consistent verbosity across services.
func verbosityFlags() []string {
	level := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
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
