package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type PlaybookRunner struct {
	Playbook string
	Forks    int
}

func (r PlaybookRunner) RunDeleteUser(ctx context.Context, inventoryPath, username string) error {
	// Build args with verbosity based on LOG_LEVEL
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
	// Inherit stdout/stderr to avoid buffering/blocks
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

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

	// Mirror the multi-host runner behavior: stream Ansible output
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

// RunDeleteUserWithSummary runs the delete action over the provided inventory
// and returns a per-host summary parsed from the Ansible recap.
// Status values: "ok", "unreachable", "failed", or "unknown".
func (r PlaybookRunner) RunDeleteUserWithSummary(ctx context.Context, inventoryPath, username string) (map[string]string, error) {
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

	// Capture output while still streaming it to logs
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &out)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errb)

	err := cmd.Run()

	// Parse recap from combined output (stdout + stderr)
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
	// Fallback: if nothing matched, leave empty map to signal unknown parsing
	return result
}
