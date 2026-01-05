package runner

// Runner wraps ansible-playbook invocations with consistent flags, logging, and parsing.
// KISS principle: small helpers, clear responsibilities, no shared mutable state.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/apenella/go-ansible/v2/pkg/execute"
	playbook "github.com/apenella/go-ansible/v2/pkg/playbook"
)

// PlaybookRunner holds minimal settings to run a playbook.
type PlaybookRunner struct {
	Playbook string
	Forks    int
}

// RunDeleteUser executes the delete-user action for hosts listed in an INI inventory.
// It streams stdout/stderr to container logs for transparency.
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
		execute.WithErrorEnrich(playbook.NewAnsiblePlaybookErrorEnrich()),
	)
	return exec.Execute(ctx)
}

// RunDeleteUserSingleHost writes a one-host inventory and runs delete-user against that host.
// This keeps the behavior explicit and simple.
func (r PlaybookRunner) RunDeleteUserSingleHost(ctx context.Context, machineName, host string, port int, sshUser, password, username string) error {
	// Write a temporary one-host inventory
	f, err := os.CreateTemp("", "inv-*.ini")
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n",
		machineName, host, port, sshUser, password)
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
		execute.WithErrorEnrich(playbook.NewAnsiblePlaybookErrorEnrich()),
	)
	return exec.Execute(ctx)
}

// RunDeleteUserWithSummary executes delete-user over the provided inventory and
// returns a per-host status parsed from the PLAY RECAP. Status values:
// "ok", "unreachable", "failed", or "unknown".
func (r PlaybookRunner) RunDeleteUserWithSummary(ctx context.Context, inventoryPath, username string) (map[string]string, error) {
	var out bytes.Buffer
	var errb bytes.Buffer

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
		execute.WithWrite(io.MultiWriter(os.Stdout, &out)),
		execute.WithWriteError(io.MultiWriter(os.Stderr, &errb)),
		execute.WithErrorEnrich(playbook.NewAnsiblePlaybookErrorEnrich()),
	)

	err := exec.Execute(ctx)

	combined := out.String() + "\n" + errb.String()
	summary := parseAnsibleRecap(combined)
	return summary, err
}

// parseAnsibleRecap extracts per-host status from Ansible PLAY RECAP lines.
// Example line:
// host1 : ok=1 changed=0 unreachable=0 failed=0 skipped=0 rescued=0 ignored=0
func parseAnsibleRecap(s string) map[string]string {
	result := map[string]string{}
	re := regexp.MustCompile(`(?m)^\s*([^\s:]+)\s*:\s*ok=(\d+)\s+changed=\d+\s+unreachable=(\d+)\s+failed=(\d+)`)
	matches := re.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		host := m[1]
		unreach := m[3]
		fail := m[4]
		switch {
		case unreach != "0":
			result[host] = "unreachable"
		case fail != "0":
			result[host] = "failed"
		default:
			result[host] = "ok"
		}
	}
	return result
}
