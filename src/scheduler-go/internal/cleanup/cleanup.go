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
	DB         *db.DB
	Runner     runner.PlaybookRunner
	TempDir    string
	BatchSize  int
}

func (c Cleaner) RunOnce(ctx context.Context) (int, error) {
	if ctx.Err() != nil { return 0, ctx.Err() }

	rows, err := db.LoadExpired(c.DB)
	if err != nil { return 0, err }
	if len(rows) == 0 { return 0, nil }

	// Group by username
	byUser := map[string][]db.ExpiredRow{}
	for _, r := range rows {
		byUser[r.User] = append(byUser[r.User], r)
	}

	total := 0
	for username, items := range byUser {
		// Process in batches
		for i := 0; i < len(items); i += c.BatchSize {
			if ctx.Err() != nil { return total, ctx.Err() }

			end := i + c.BatchSize
			if end > len(items) { end = len(items) }
			chunk := items[i:end]

			inv, err := c.writeInventory(chunk)
			if err != nil { return total, err }
			func() {
				defer os.Remove(inv)
				_ = c.Runner.RunDeleteUser(ctx, inv, username)
			}()
		}

		// Clear DB regardless of Ansible result
		pairs := make([][2]int, 0, len(items))
		for _, r := range items {
			pairs = append(pairs, [2]int{r.ResID, r.MID})
		}
		if err := db.ClearReservationsAndRelease(c.DB, pairs); err != nil {
			return total, err
		}
		total += len(items)
	}

	return total, nil
}

func (c Cleaner) writeInventory(chunk []db.ExpiredRow) (string, error) {
	ts := time.Now().UnixNano()
	path := filepath.Join(c.TempDir, fmt.Sprintf("inv-%d.ini", ts))
	f, err := os.Create(path)
	if err != nil { return "", err }
	defer f.Close()

	for _, r := range chunk {
		line := fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n",
			r.Name, r.Host, r.Port, r.SSHUser, r.Pass)
		if _, err := f.WriteString(line); err != nil { return "", err }
	}
	return path, nil
}
