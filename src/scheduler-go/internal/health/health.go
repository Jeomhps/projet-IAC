// Health module: simple, focused, and well-documented.
// - Performs SSH reachability checks.
// - Updates DB flags (enabled) and timestamps (last_seen_at).
// - Triggers opportunistic cleanup when a machine comes back online.
// Keep it KISS: small helpers, clear control flow, and idempotent DB updates.
package health

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"os/exec"

	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/db"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/runner"
)

// Checker performs SSH reachability checks against registered machines and updates DB fields.
// Behavior:
// - Success: update last_seen_at; if it was disabled, enable it and attempt opportunistic cleanup.
// - Failure: set enabled=false to hide it from users and avoid useless retries until it recovers.
// Notes:
// - Concurrency and per-host timeouts are configurable.
// - All DB operations are idempotent to handle races safely.
type Checker struct {
	DB          *db.DB
	Runner      runner.PlaybookRunner
	Concurrency int           // number of concurrent SSH checks
	Timeout     time.Duration // per-host timeout
}

// Stats summarizes one run of the health check.
// Keep fields human-friendly for easy logging and dashboards.
type Stats struct {
	Total       int // total machines processed
	Reachable   int // SSH connected successfully
	Unreachable int // SSH failed
	Disabled    int // machines disabled (enabled->false) during this run
	ReEnabled   int // machines re-enabled (enabled->true) during this run
}

// RunOnce executes an SSH health check pass for all machines.
// It respects context cancellation. On success/failure it updates the DB:
// - Success: TouchMachineLastSeen and SetMachineEnabled(..., true) if it was disabled
// - Failure: SetMachineEnabled(..., false)
func (c Checker) RunOnce(ctx context.Context) (Stats, error) {
	var stats Stats

	if ctx.Err() != nil {
		return stats, ctx.Err()
	}
	rows, err := db.LoadMachines(c.DB)
	if err != nil {
		return stats, err
	}
	if len(rows) == 0 {
		return stats, nil
	}

	// Use safe defaults to keep behavior predictable when unset.
	cc := c.Concurrency
	if cc <= 0 {
		cc = 10
	}
	to := c.Timeout
	if to <= 0 {
		to = 10 * time.Second
	}

	jobs := make(chan db.MachineRow)
	var wg sync.WaitGroup

	var reachable int32
	var unreachable int32
	var disabled int32
	var reenabled int32
	var total int32

	// Worker: probes hosts and applies DB side-effects based on the outcome.
	// Idempotent writes and context checks keep this robust and easy to reason about.
	worker := func() {
		defer wg.Done()
		for m := range jobs {
			if ctx.Err() != nil {
				return
			}
			atomic.AddInt32(&total, 1)

			ok := trySSH(ctx, m, to)
			if ok {
				// On success: update last_seen_at and re-enable if needed
				if err := db.TouchMachineLastSeen(c.DB, m.ID); err != nil {
					log.Printf("health: touch last_seen_at failed for %s (%s:%d): %v", m.Name, m.Host, m.Port, err)
				}
				// Only count as re-enabled if the DB row actually changed (prevents overcounting).
				if changed, err := db.EnableIfDisabled(c.DB, m.ID); err != nil {
					log.Printf("health: set enabled=true failed for %s (%s:%d): %v", m.Name, m.Host, m.Port, err)
				} else if changed {
					atomic.AddInt32(&reenabled, 1)
					// Opportunistic cleanup: machine just re-enabled, try to clear any expired reservations now.
					if err := opportunisticCleanup(ctx, c.DB, c.Runner, m); err != nil && ctx.Err() == nil {
						log.Printf("health: opportunistic cleanup failed for %s: %v", m.Name, err)
					}
				}
				atomic.AddInt32(&reachable, 1)
			} else {
				// On failure: disable machine
				if m.Enabled {
					if err := db.SetMachineEnabled(c.DB, m.ID, false); err != nil {
						log.Printf("health: set enabled=false failed for %s (%s:%d): %v", m.Name, m.Host, m.Port, err)
					} else {
						atomic.AddInt32(&disabled, 1)
					}
				}
				atomic.AddInt32(&unreachable, 1)
			}
		}
	}

	// Start workers
	wg.Add(cc)
	for i := 0; i < cc; i++ {
		go worker()
	}

	// Feed jobs
	for _, m := range rows {
		// Early exit if context is canceled
		if ctx.Err() != nil {
			break
		}
		jobs <- m
	}
	close(jobs)

	wg.Wait()

	stats.Total = int(atomic.LoadInt32(&total))
	stats.Reachable = int(atomic.LoadInt32(&reachable))
	stats.Unreachable = int(atomic.LoadInt32(&unreachable))
	stats.Disabled = int(atomic.LoadInt32(&disabled))
	stats.ReEnabled = int(atomic.LoadInt32(&reenabled))
	return stats, nil
}

// trySSH returns true if we can establish an SSH connection using password auth.
// Implementation notes:
// - Uses sshpass + ssh to avoid bringing extra crypto deps; this aligns with the container toolchain.
// - Host key checking is disabled intentionally (health probe, not trust establishment).
// - Respects per-host timeout via context.
func trySSH(ctx context.Context, m db.MachineRow, timeout time.Duration) bool {
	addr := fmt.Sprintf("%s@%s", m.SSHUser, m.Host)

	// Derive a bounded context for each attempt
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use sshpass + ssh to probe connectivity
	args := []string{
		"-p", m.Pass,
		"ssh",
		"-o", "BatchMode=no",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ConnectTimeout=%d", int(timeout.Seconds())),
		"-p", fmt.Sprintf("%d", m.Port),
		addr,
		"true",
	}
	cmd := exec.CommandContext(cctx, "sshpass", args...)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

// opportunisticCleanup deletes expired reservation users for a machine that just came back.
// KISS: best-effort per-host deletes with idempotent DB clears on success.
// If Ansible fails, leave the reservation; regular cleanup will retry later.
func opportunisticCleanup(ctx context.Context, d *db.DB, pr runner.PlaybookRunner, m db.MachineRow) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	rows, err := db.LoadExpiredForMachine(d, m.ID)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	for _, row := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Best-effort delete; logs will follow runner's configured behavior
		if err := pr.RunDeleteUserSingleHost(ctx, row.Name, row.Host, row.Port, row.SSHUser, row.Pass, row.User); err == nil {
			_ = db.ClearReservationsAndRelease(d, [][2]int{{row.ResID, row.MID}})
		}
	}
	return nil
}

// runAnsibleDeleteSingle runs the delete user playbook for a single host.
