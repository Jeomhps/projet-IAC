package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/db"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/runner"
)

type Cleaner struct {
	DB        *db.DB
	Runner    runner.PlaybookRunner
	TempDir   string
	BatchSize int
}

func (c Cleaner) RunOnce(ctx context.Context) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}

	rows, err := db.LoadExpired(c.DB)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}

	// Group by username
	byUser := map[string][]db.ExpiredRow{}
	for _, r := range rows {
		byUser[r.User] = append(byUser[r.User], r)
	}

	total := 0
	for username, items := range byUser {
		// Process per-host to decide DB changes based on Ansible outcome
		for _, r := range items {
			if ctx.Err() != nil {
				return total, ctx.Err()
			}

			inv, err := c.writeInventory([]db.ExpiredRow{r})
			if err != nil {
				return total, err
			}

			// Run deletion for this single host
			runErr := c.Runner.RunDeleteUser(ctx, inv, username)
			_ = os.Remove(inv)

			if runErr == nil {
				// Clear reservation only on success
				if err := db.ClearReservationsAndRelease(c.DB, [][2]int{{r.ResID, r.MID}}); err != nil {
					return total, err
				}
				total++
			} else {
				// Disable machine on failure for later retry when reachable
				_ = db.SetMachineEnabled(c.DB, r.MID, false)
			}
		}
	}

	return total, nil
}

func (c Cleaner) writeInventory(chunk []db.ExpiredRow) (string, error) {
	ts := time.Now().UnixNano()
	path := filepath.Join(c.TempDir, fmt.Sprintf("inv-%d.ini", ts))
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for _, r := range chunk {
		line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n",
			r.Name, r.Host, r.Port, r.SSHUser, r.Pass)
		if _, err := f.WriteString(line); err != nil {
			return "", err
		}
	}
	return path, nil
}
