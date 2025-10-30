package reconcile

// Reconciliation pass:
// - If a reserved machine is disabled but still within reservation, allocate a replacement from the spare pool.
// - If the original machine comes back (enabled again) before reservation expiry, release the replacement.
// - When allocating a replacement, create the user account on the replacement host using the stored hashed_password.
//   If user creation fails, rollback the DB changes so the replacement is not kept.
// Assumptions and scope:
// - "Spare pool" machines are flagged via machines.spare_pool=1. Selection uses enabled=1, online=1, reserved=0.
// - Replacement reservations are tracked with reservations.replacement_for_machine_id = <primary machine id>.
//
// Safety and idempotency:
// - All mutating steps are wrapped in small, targeted transactions.
// - This can run concurrently with other maintenance; WHERE clauses and conditions avoid double-allocation.

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/db"
	"github.com/Jeomhps/projet-IAC/scheduler-go/internal/runner"
)

type Reconciler struct {
	DB               *db.DB
	Runner           runner.PlaybookRunner
	SparePoolPercent int
}

// RunOnce executes a reconciliation pass:
// 1) Release replacement reservations for primaries that are back online (enabled).
// 2) Allocate replacements for primaries that are disabled and lack an enabled replacement.
func (r Reconciler) RunOnce(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// 0) Enforce spare pool size based on SparePercent (percentage of enabled+online machines).
	if err := r.enforceSparePool(ctx); err != nil {
		log.Printf("reconcile: spare pool enforcement failed: %v", err)
	}

	// 1) Release replacements whose primary machine is enabled again.
	if err := r.releaseReplacementsForRecoveredPrimaries(ctx); err != nil {
		return err
	}

	// 2) Allocate replacements from spare pool for primaries that have no enabled machine.
	if err := r.allocateNeededReplacements(ctx); err != nil {
		return err
	}

	return nil
}

// releaseReplacementsForRecoveredPrimaries removes replacement reservations when
// the associated primary machine has recovered (enabled=1), and frees the spare machine.
func (r Reconciler) releaseReplacementsForRecoveredPrimaries(ctx context.Context) error {
	type row struct {
		ResID        int        `db:"res_id"`
		ReplaceMID   int        `db:"replace_mid"`
		Username     string     `db:"username"`
		PrimaryMID   int        `db:"primary_mid"`
		PrimaryEn    bool       `db:"primary_enabled"`
		ReplaceUntil *time.Time `db:"reserved_until"`
		RName        string     `db:"r_name"`
		RHost        string     `db:"r_host"`
		RPort        int        `db:"r_port"`
		RUser        string     `db:"r_user"`
		RPass        string     `db:"r_pass"`
	}
	var rows []row

	// Only consider active replacement reservations (time window check).
	q := `
SELECT rr.id           AS res_id,
       rr.machine_id   AS replace_mid,
       rr.username     AS username,
       pr.machine_id   AS primary_mid,
       pm.enabled      AS primary_enabled,
       rr.reserved_until,
       rm.name         AS r_name,
       rm.host         AS r_host,
       rm.port         AS r_port,
       rm.user         AS r_user,
       rm.password     AS r_pass
FROM reservations rr
JOIN machines pm ON pm.id = rr.replacement_for_machine_id
JOIN reservations pr ON pr.machine_id = rr.replacement_for_machine_id
                    AND pr.username = rr.username
                    AND (pr.reserved_until IS NULL OR pr.reserved_until > UTC_TIMESTAMP())
JOIN machines rm ON rm.id = rr.machine_id
WHERE (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
  AND pm.enabled = 1
`
	if err := r.DB.SelectContext(ctx, &rows, q); err != nil {
		return fmt.Errorf("reconcile query (release replacements): %w", err)
	}
	log.Printf("reconcile: replacements_to_release=%d", len(rows))
	released := 0
	delOK := 0
	delFail := 0
	if len(rows) == 0 {
		log.Printf("reconcile: released=0 delete_ok=0 delete_fail=0")
		return nil
	}

	for _, x := range rows {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Best-effort: delete user from the replacement host before releasing it
		if r.Runner.Playbook != "" {
			if err := r.Runner.RunDeleteUserSingleHost(ctx, x.RName, x.RHost, x.RPort, x.RUser, x.RPass, x.Username); err != nil {
				delFail++
				log.Printf("reconcile: delete-user on replacement failed for user=%s machine=%s: %v", x.Username, x.RName, err)
			} else {
				delOK++
				log.Printf("reconcile: delete-user on replacement ok for user=%s machine=%s", x.Username, x.RName)
			}
		} else {
			if err := runDeleteUserSingleHost(ctx, x.RName, x.RHost, x.RPort, x.RUser, x.RPass, x.Username); err != nil {
				delFail++
				log.Printf("reconcile: delete-user on replacement failed for user=%s machine=%s: %v", x.Username, x.RName, err)
			} else {
				delOK++
				log.Printf("reconcile: delete-user on replacement ok for user=%s machine=%s", x.Username, x.RName)
			}
		}
		// In a transaction:
		// - Delete the replacement reservation.
		// - Free the replacement machine (reserved flags -> 0).
		tx, err := r.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		if err != nil {
			return fmt.Errorf("tx begin (release): %w", err)
		}
		ok := false
		defer func() {
			if !ok {
				_ = tx.Rollback()
			}
		}()

		// Double-check still valid within TX (avoid races):
		var still int
		err = tx.GetContext(ctx, &still, `
SELECT COUNT(1)
FROM reservations rr
JOIN machines pm ON pm.id = rr.replacement_for_machine_id
WHERE rr.id=?
  AND (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
  AND pm.enabled=1
`, x.ResID)
		if err != nil {
			return fmt.Errorf("tx recheck (release): %w", err)
		}
		if still == 0 {
			_ = tx.Rollback()
			continue
		}

		if _, err := tx.ExecContext(ctx, `DELETE FROM reservations WHERE id=?`, x.ResID); err != nil {
			return fmt.Errorf("delete replacement reservation: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE machines
SET reserved=0, reserved_by=NULL, reserved_until=NULL
WHERE id=?`, x.ReplaceMID); err != nil {
			return fmt.Errorf("free replacement machine: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("tx commit (release): %w", err)
		}
		ok = true
		released++
		log.Printf("reconcile: released replacement (res_id=%d, machine_id=%d) for primary mid=%d user=%s",
			x.ResID, x.ReplaceMID, x.PrimaryMID, x.Username)
	}
	log.Printf("reconcile: release summary released=%d delete_ok=%d delete_fail=%d", released, delOK, delFail)
	return nil
}

// allocateNeededReplacements finds primaries that are disabled and have no enabled replacement,
// then allocates a spare machine and creates a replacement reservation mirroring the primary's expiration.
func (r Reconciler) allocateNeededReplacements(ctx context.Context) error {
	type needRow struct {
		Username       string     `db:"username"`
		PrimaryMID     int        `db:"primary_mid"`
		Until          *time.Time `db:"reserved_until"`
		HashedPassword *string    `db:"hashed_password"`
	}

	// Identify primaries which are active (time-wise), are not enabled,
	// and do not currently have an enabled replacement. We only allocate IF no replacement exists at all.
	// This avoids piling up multiple replacements per primary.
	var needs []needRow
	q := `
SELECT pr.username,
       pr.machine_id AS primary_mid,
       pr.reserved_until,
       pr.hashed_password
FROM reservations pr
JOIN machines pm ON pm.id = pr.machine_id
LEFT JOIN reservations rr
       ON rr.replacement_for_machine_id = pr.machine_id
      AND rr.username = pr.username
      AND (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
LEFT JOIN machines rm ON rm.id = rr.machine_id
WHERE (pr.reserved_until IS NULL OR pr.reserved_until > UTC_TIMESTAMP())
  AND pr.replacement_for_machine_id IS NULL
  AND pm.enabled = 0
  AND rr.id IS NULL
`
	if err := r.DB.SelectContext(ctx, &needs, q); err != nil {
		return fmt.Errorf("reconcile query (needs): %w", err)
	}
	log.Printf("reconcile: needs_replacement=%d", len(needs))
	allocated := 0
	failed := 0
	if len(needs) == 0 {
		log.Printf("reconcile: allocated=0 failed=0")
		return nil
	}

	for _, n := range needs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := r.allocateOne(ctx, n.Username, n.PrimaryMID, n.Until, n.HashedPassword); err != nil {
			// Best effort: log and continue to the next need.
			failed++
			log.Printf("reconcile: allocate replacement failed for user=%s primary_mid=%d: %v", n.Username, n.PrimaryMID, err)
			continue
		}
		allocated++
		log.Printf("reconcile: allocated replacement for user=%s primary_mid=%d", n.Username, n.PrimaryMID)
	}
	return nil
}

// allocateOne reserves one spare-pool machine for the given user as a replacement for the primary MID.
// It provisions the user on the replacement host with the stored hashed_password before committing the TX.
func (r Reconciler) allocateOne(ctx context.Context, username string, primaryMID int, until *time.Time, hashedPassword *string) error {
	tx, err := r.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("tx begin (allocate): %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = tx.Rollback()
		}
	}()

	// Verify the primary still needs replacement within TX (avoid races):
	var still int
	err = tx.GetContext(ctx, &still, `
SELECT COUNT(1)
FROM reservations pr
JOIN machines pm ON pm.id = pr.machine_id
LEFT JOIN reservations rr
       ON rr.replacement_for_machine_id = pr.machine_id
      AND rr.username = pr.username
      AND (rr.reserved_until IS NULL OR rr.reserved_until > UTC_TIMESTAMP())
WHERE (pr.reserved_until IS NULL OR pr.reserved_until > UTC_TIMESTAMP())
  AND pr.replacement_for_machine_id IS NULL
  AND pm.enabled = 0
  AND pr.machine_id = ?
  AND pr.username = ?
  AND rr.id IS NULL
`, primaryMID, username)
	if err != nil || still == 0 {
		_ = tx.Rollback()
		if err != nil {
			return fmt.Errorf("tx recheck (allocate): %w", err)
		}
		return nil
	}

	// Pick a spare machine: enabled=1, online=1, reserved=0, spare_pool=1
	type mh struct {
		Name string `db:"name"`
		Host string `db:"host"`
		Port int    `db:"port"`
		User string `db:"user"`
		Pass string `db:"password"`
	}
	var spareMID int
	var spare mh
	err = tx.GetContext(ctx, &spareMID, `
SELECT id
FROM machines
WHERE spare_pool=1 AND enabled=1 AND online=1 AND reserved=0
ORDER BY reserve_fail_count ASC, id ASC
LIMIT 1 FOR UPDATE
`)
	if err != nil {
		if err == sql.ErrNoRows {
			_ = tx.Rollback()
			return fmt.Errorf("no spare machines available")
		}
		return fmt.Errorf("select spare: %w", err)
	}
	if err := tx.GetContext(ctx, &spare, `SELECT name,host,port,user,password FROM machines WHERE id=?`, spareMID); err != nil {
		return fmt.Errorf("load spare details: %w", err)
	}

	// Resolve user_id
	var uid int
	if err := tx.GetContext(ctx, &uid, `SELECT id FROM users WHERE username=?`, username); err != nil {
		return fmt.Errorf("lookup user id: %w", err)
	}

	// Reserve the spare machine
	if until != nil {
		if _, err := tx.ExecContext(ctx, `
UPDATE machines
SET reserved=1, reserved_by=?, reserved_until=?
WHERE id=? AND reserved=0
`, username, until.UTC(), spareMID); err != nil {
			return fmt.Errorf("reserve spare: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
UPDATE machines
SET reserved=1, reserved_by=?, reserved_until=NULL
WHERE id=? AND reserved=0
`, username, spareMID); err != nil {
			return fmt.Errorf("reserve spare: %w", err)
		}
	}

	// Insert replacement reservation linked to the primary (includes hashed_password for provisioning)
	if until != nil {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO reservations (machine_id, user_id, username, reserved_until, hashed_password, replacement_for_machine_id)
VALUES (?,?,?,?,?,?)
`, spareMID, uid, username, until.UTC(), hashedPassword, primaryMID); err != nil {
			return fmt.Errorf("insert replacement reservation: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO reservations (machine_id, user_id, username, reserved_until, hashed_password, replacement_for_machine_id)
VALUES (?,?,?,?,?,?)
`, spareMID, uid, username, nil, hashedPassword, primaryMID); err != nil {
			return fmt.Errorf("insert replacement reservation: %w", err)
		}
	}

	// Provision the user on the replacement host using the stored hashed_password.
	if hashedPassword == nil || *hashedPassword == "" {
		return fmt.Errorf("no hashed_password available to provision user on replacement machine")
	}
	if err := runCreateUserSingleHost(ctx, spare.Name, spare.Host, spare.Port, spare.User, spare.Pass, username, *hashedPassword); err != nil {
		// Rollback to avoid leaving a replacement allocated without a user provisioned.
		_ = tx.Rollback()
		return fmt.Errorf("ansible create-user failed on replacement host: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("tx commit (allocate): %w", err)
	}
	ok = true

	log.Printf("reconcile: allocated replacement spare_mid=%d for user=%s primary_mid=%d (until=%v)",
		spareMID, username, primaryMID, until)
	return nil
}

// runCreateUserSingleHost provisions the user on a single host by invoking ansible-playbook.
// It builds a one-host inventory and uses the hashed_password provided by the API at reservation time.
func runCreateUserSingleHost(ctx context.Context, machineName, host string, port int, sshUser, password, username, hashedPassword string) error {
	// Write temp inventory
	dir := os.TempDir()
	fn := filepath.Join(dir, fmt.Sprintf("inv-%d.ini", time.Now().UnixNano()))
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n", machineName, host, port, sshUser, password)); err != nil {
		_ = f.Close()
		_ = os.Remove(fn)
		return err
	}
	_ = f.Close()
	defer os.Remove(fn)

	playbook := os.Getenv("ANSIBLE_PLAYBOOK")
	if playbook == "" {
		playbook = "/app/playbooks/create-users.yml"
	}
	forks := os.Getenv("ANSIBLE_FORKS")
	if forks == "" {
		forks = "5"
	}

	args := []string{
		"ansible-playbook",
		"-f", forks,
		"-i", fn,
		playbook,
		"--extra-vars", fmt.Sprintf("username=%s hashed_password=%s user_action=create ansible_ssh_timeout=15", username, hashedPassword),
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	// Stream to container logs; verbosity is controlled by env set by main (ANSIBLE_VERBOSITY, etc.)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDeleteUserSingleHost removes the user on a single host by invoking ansible-playbook.
// It builds a one-host inventory and uses the existing playbook with user_action=delete.
func runDeleteUserSingleHost(ctx context.Context, machineName, host string, port int, sshUser, password, username string) error {
	// Write temp inventory
	dir := os.TempDir()
	fn := filepath.Join(dir, fmt.Sprintf("inv-%d.ini", time.Now().UnixNano()))
	f, err := os.Create(fn)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n", machineName, host, port, sshUser, password)); err != nil {
		_ = f.Close()
		_ = os.Remove(fn)
		return err
	}
	_ = f.Close()
	defer os.Remove(fn)

	playbook := os.Getenv("ANSIBLE_PLAYBOOK")
	if playbook == "" {
		playbook = "/app/playbooks/create-users.yml"
	}
	forks := os.Getenv("ANSIBLE_FORKS")
	if forks == "" {
		forks = "5"
	}

	args := []string{
		"ansible-playbook",
		"-f", forks,
		"-i", fn,
		playbook,
		"--extra-vars", fmt.Sprintf("username=%s user_action=delete ansible_ssh_timeout=15", username),
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// enforceSparePool adjusts spare_pool flags to maintain a target of free spare machines
// based on SparePercent% of enabled+online machines. It only promotes/demotes available machines
// (enabled=1 AND online=1 AND reserved=0) and leaves reserved machines untouched.
func (r Reconciler) enforceSparePool(ctx context.Context) error {
	if r.SparePoolPercent <= 0 {
		log.Printf("reconcile: spare_pool disabled (SPARE_POOL_PERCENT=0)")
		return nil
	}

	// Total eligible machines (enabled + online)
	var total int
	if err := r.DB.GetContext(ctx, &total, `SELECT COUNT(1) FROM machines WHERE enabled=1 AND online=1`); err != nil {
		return err
	}
	if total <= 0 {
		return nil
	}

	// Desired number of free spares (round down)
	desired := (total * r.SparePoolPercent) / 100
	if desired < 0 {
		desired = 0
	}

	// Current count of free spare machines
	var current int
	if err := r.DB.GetContext(ctx, &current, `SELECT COUNT(1) FROM machines WHERE spare_pool=1 AND enabled=1 AND online=1 AND reserved=0`); err != nil {
		return err
	}

	log.Printf("reconcile: spare_pool total_enabled_online=%d desired=%d current_free_spares=%d", total, desired, current)
	switch {
	case current < desired:
		need := desired - current
		log.Printf("reconcile: spare_pool promote_needed=%d", need)
		// Promote available, non-spare machines into the spare pool
		var ids []int
		if err := r.DB.SelectContext(ctx, &ids, `
SELECT id
FROM machines
WHERE spare_pool=0 AND enabled=1 AND online=1 AND reserved=0
ORDER BY reserve_fail_count ASC, id ASC
LIMIT ?`, need); err != nil {
			return err
		}
		if len(ids) > 0 {
			tx, err := r.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if err != nil {
				return err
			}
			ok := false
			defer func() {
				if !ok {
					_ = tx.Rollback()
				}
			}()
			for _, id := range ids {
				if _, err := tx.ExecContext(ctx, `UPDATE machines SET spare_pool=1 WHERE id=? AND spare_pool=0 AND enabled=1 AND online=1 AND reserved=0`, id); err != nil {
					return err
				}
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			ok = true
			log.Printf("reconcile: spare_pool promoted=%d", len(ids))
		}
	case current > desired:
		drop := current - desired
		log.Printf("reconcile: spare_pool demote_needed=%d", drop)
		// Demote extra free spares back to normal
		var ids []int
		if err := r.DB.SelectContext(ctx, &ids, `
SELECT id
FROM machines
WHERE spare_pool=1 AND reserved=0
ORDER BY reserve_fail_count DESC, id DESC
LIMIT ?`, drop); err != nil {
			return err
		}
		if len(ids) > 0 {
			tx, err := r.DB.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
			if err != nil {
				return err
			}
			ok := false
			defer func() {
				if !ok {
					_ = tx.Rollback()
				}
			}()
			for _, id := range ids {
				if _, err := tx.ExecContext(ctx, `UPDATE machines SET spare_pool=0 WHERE id=? AND spare_pool=1 AND reserved=0`, id); err != nil {
					return err
				}
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			ok = true
			log.Printf("reconcile: spare_pool demoted=%d", len(ids))
		}
	}

	return nil
}
