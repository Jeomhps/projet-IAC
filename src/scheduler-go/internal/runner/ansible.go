package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type PlaybookRunner struct {
	Playbook string
	Forks    int
}

func (r PlaybookRunner) RunDeleteUser(ctx context.Context, inventoryPath, username string) error {
	args := []string{
		"ansible-playbook",
		"-f", fmt.Sprintf("%d", r.Forks),
		"-i", inventoryPath,
		r.Playbook,
		"--extra-vars", fmt.Sprintf("username=%s user_action=delete ansible_ssh_timeout=15", username),
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// Inherit stdout/stderr to avoid buffering/blocks
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
