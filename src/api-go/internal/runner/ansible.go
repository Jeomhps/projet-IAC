package runner

import (
	"context"
	"fmt"
	"os"

	"github.com/apenella/go-ansible/v2/pkg/execute"
	playbook "github.com/apenella/go-ansible/v2/pkg/playbook"
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
	opts := &playbook.AnsiblePlaybookOptions{
		Inventory: inventoryPath,
		Forks:     fmt.Sprintf("%d", r.Forks),
		ExtraVars: map[string]interface{}{
			"username":            username,
			"user_action":         "delete",
			"ansible_ssh_timeout": 15,
		},
	}

	cmd := playbook.NewAnsiblePlaybookCmd(
		playbook.WithPlaybooks(r.Playbook),
		playbook.WithPlaybookOptions(opts),
	)

	exec := execute.NewDefaultExecute(
		execute.WithCmd(cmd),
		execute.WithWrite(os.Stdout),
		execute.WithWriteError(os.Stderr),
		execute.WithEnvVars(map[string]string{
			"ANSIBLE_FORCE_COLOR": "1",
		}),
	)

	return exec.Execute(ctx)
}

// RunDeleteUserSingleHost writes a temporary one-host inventory and runs the
// delete-user action against that single host, streaming logs as usual.
func (r PlaybookRunner) RunDeleteUserSingleHost(ctx context.Context, machineName, host string, port int, sshUser, password, username string) error {
	f, err := os.CreateTemp("", "inv-*.ini")
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n", machineName, host, port, sshUser, password)
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return err
	}
	_ = f.Close()
	defer os.Remove(f.Name())

	opts := &playbook.AnsiblePlaybookOptions{
		Inventory: f.Name(),
		Forks:     fmt.Sprintf("%d", r.Forks),
		ExtraVars: map[string]interface{}{
			"username":            username,
			"user_action":         "delete",
			"ansible_ssh_timeout": 15,
		},
	}

	cmd := playbook.NewAnsiblePlaybookCmd(
		playbook.WithPlaybooks(r.Playbook),
		playbook.WithPlaybookOptions(opts),
	)

	exec := execute.NewDefaultExecute(
		execute.WithCmd(cmd),
		execute.WithWrite(os.Stdout),
		execute.WithWriteError(os.Stderr),
		execute.WithEnvVars(map[string]string{
			"ANSIBLE_FORCE_COLOR": "1",
		}),
	)

	return exec.Execute(ctx)
}

// Note: Verbosity is controlled via environment (e.g., ANSIBLE_VERBOSITY) based on LOG_LEVEL mapping.
// We do not add -v flags explicitly here; go-ansible will honor the environment.
