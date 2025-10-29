package reservations

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
)

// Create reserves N machines for the authenticated user, runs the create-user playbook,
// and persists reservations on success.
//
// KISS + correctness via "reserve-first" transactional flow:
// 1) In a TX: lock/select eligible machines and mark them reserved.
// 2) Run Ansible to create the user on those machines.
// 3) On success: insert reservation rows. On failure: rollback "reserved" flags and apply backoff/quarantine.
func (h *Handler) Create(c *gin.Context) {
	user := c.GetString("user")

	// Minimal request validation
	var in struct {
		Count    int    `json:"count"`
		Duration int    `json:"duration_minutes"`
		Password string `json:"reservation_password"`
		Username string `json:"username"` // rejected
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Count <= 0 || in.Duration <= 0 || strings.TrimSpace(in.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	if strings.TrimSpace(in.Username) != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "username must not be provided"})
		return
	}

	// Lookup authenticated user's ID (needed for reservations table)
	var uid int
	if err := h.db.Get(&uid, "SELECT id FROM users WHERE username=?", user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Reserve-first transactional flow to prevent collisions across concurrent requests.
	tx := h.db.MustBegin()
	var ids []int
	// Lock eligible rows and choose N with a simple ordering that prioritizes less-failing hosts.
	if err := tx.Select(&ids, `
		SELECT id
		FROM machines
		WHERE enabled=1 AND online=1 AND reserved=0
		  AND (quarantine_until IS NULL OR quarantine_until <= UTC_TIMESTAMP())
		ORDER BY reserve_fail_count ASC, id ASC
		LIMIT ? FOR UPDATE`, in.Count); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if len(ids) < in.Count {
		_ = tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"error": "not_enough_available", "available": len(ids)})
		return
	}
	until := time.Now().UTC().Add(time.Duration(in.Duration) * time.Minute)

	// Mark selected machines reserved within the TX
	ph := make([]string, len(ids))
	upArgs := make([]any, 0, 2+len(ids))
	upArgs = append(upArgs, user, until)
	for i := range ids {
		ph[i] = "?"
		upArgs = append(upArgs, ids[i])
	}
	if _, err := tx.Exec("UPDATE machines SET reserved=1,reserved_by=?,reserved_until=? WHERE id IN ("+strings.Join(ph, ",")+") AND reserved=0", upArgs...); err != nil {
		_ = tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Load reserved machine rows to build inventory
	var reserved []db.Machine
	selArgs := make([]any, 0, len(ids))
	for i := range ids {
		selArgs = append(selArgs, ids[i])
	}
	query := "SELECT * FROM machines WHERE id IN (" + strings.Join(ph, ",") + ") ORDER BY id ASC"
	if err := h.db.Select(&reserved, query, selArgs...); err != nil {
		// Revert reserved flags if we failed to load rows
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Build one-off inventory
	hosts := make([]InventoryHost, 0, len(reserved))
	for _, m := range reserved {
		hosts = append(hosts, InventoryHost{
			Name:     m.Name,
			Host:     m.Host,
			Port:     m.Port,
			User:     m.User,
			Password: m.Password,
		})
	}
	inv, cleanup, err := WriteTempInventory(hosts)
	if err != nil {
		// Revert reserved flags on internal errors
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	defer cleanup()

	// Hash password with OpenSSL SHA-512-crypt ($6$)
	hashed, err := HashSHA512Crypt(in.Password)
	if err != nil || strings.TrimSpace(hashed) == "" {
		// Revert reserved flags
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "message": "hashing failed"})
		return
	}

	// Prepare ansible-playbook invocation with consistent verbosity and forks
	level := LogLevel()
	forks := EnvForks(10)
	playbook := strings.TrimSpace(os.Getenv("ANSIBLE_PLAYBOOK"))
	if playbook == "" {
		playbook = "/app/playbooks/create-users.yml"
	}
	args := BuildAnsibleArgs(level, forks, inv, playbook, fmt.Sprintf("username=%s hashed_password=%s user_action=create", user, hashed))
	cmd := exec.Command(args[0], args[1:]...)
	var stderr bytes.Buffer

	// Stream logs (optional) and redact passwords if logging inventory
	if StreamAnsible(level) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if invb, _ := os.ReadFile(inv); len(invb) > 0 {
			log.Printf("[ansible] inventory (create):\n%s", SanitizeInventory(string(invb)))
		}
	} else {
		cmd.Stderr = &stderr
	}

	// Run ansible-playbook
	if err := cmd.Run(); err != nil {
		// Revert reserved flags and apply backoff/quarantine on failure
		txr := h.db.MustBegin()
		_, _ = txr.Exec(`
			UPDATE machines
			SET reserved=0,
				reserved_by=NULL,
				reserved_until=NULL,
				reserve_fail_count=reserve_fail_count+1,
				quarantine_until=DATE_ADD(UTC_TIMESTAMP(), INTERVAL LEAST(60, (reserve_fail_count+1)*5) MINUTE)
			WHERE id IN (`+strings.Join(ph, ",")+`)`, selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ansible_failed", "details": strings.TrimSpace(stderr.String())})
		return
	}

	// Persist reservations on success (machines are already marked reserved)
	txi := h.db.MustBegin()
	for _, m := range reserved {
		txi.MustExec("INSERT INTO reservations (machine_id,user_id,username,reserved_until) VALUES (?,?,?,?)", m.ID, uid, user, until)
	}
	if err := txi.Commit(); err != nil {
		// In the unlikely event of insert failure, rollback reservation flags
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Shape response: list of machines and expiration
	out := make([]gin.H, 0, len(reserved))
	for _, m := range reserved {
		out = append(out, gin.H{"machine": m.Name, "host": m.Host, "port": m.Port})
	}
	c.JSON(http.StatusCreated, gin.H{
		"machines":         out,
		"reserved_until":   until.UTC().Format(time.RFC3339),
		"duration_minutes": in.Duration,
	})
}
