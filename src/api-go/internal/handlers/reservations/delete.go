package reservations

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/gin-gonic/gin"
)

// Delete removes a reservation by id.
// - Admins can delete any reservation.
// - Non-admins can only delete their own reservation.
// KISS behavior: run the delete-user playbook best-effort, then clear the DB regardless.
// This ensures users are unblocked even if a machine is temporarily unreachable.
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	// Load reservation and joined machine fields we need to build a single-host inventory
	type row struct {
		ID       int    `db:"id"`
		Username string `db:"username"`
		MID      int    `db:"machine_id"`
		MName    string `db:"name"`
		Host     string `db:"host"`
		Port     int    `db:"port"`
		User     string `db:"user"`
		Pass     string `db:"password"`
	}
	var x row
	if err := h.db.Get(&x, `SELECT r.id,r.username,m.id as machine_id,m.name,m.host,m.port,m.user,m.password
		FROM reservations r JOIN machines m ON m.id=r.machine_id WHERE r.id=?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	// Authorization: non-admins can only delete their own reservation
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// Prepare a one-host inventory for the target machine
	host := InventoryHost{
		Name:     x.MName,
		Host:     x.Host,
		Port:     x.Port,
		User:     x.User,
		Password: x.Pass,
	}
	inv, cleanup, err := WriteTempInventory([]InventoryHost{host})
	if err == nil { // only log inventory if it was created
		defer cleanup()
	}

	// Prepare ansible-playbook invocation with consistent verbosity and forks.
	// Best effort: we run it and ignore the error for DB cleanup below.
	level := LogLevel()
	forks := EnvForks(10)
	playbook := strings.TrimSpace(os.Getenv("ANSIBLE_PLAYBOOK"))
	if playbook == "" {
		playbook = "/app/playbooks/create-users.yml"
	}
	args := BuildAnsibleArgs(level, forks, inv, playbook, fmt.Sprintf("username=%s user_action=delete", x.Username))
	cmd := exec.Command(args[0], args[1:]...)
	var stderr bytes.Buffer

	// Stream logs in debug/trace, otherwise capture stderr only for concise error reporting
	if StreamAnsible(level) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if invb, _ := os.ReadFile(inv); len(invb) > 0 {
			// Never log raw passwords
			_ = os.Setenv("ANSIBLE_FORCE_COLOR", "1")
			// Redact sensitive data before logging
			safeInv := SanitizeInventory(string(invb))
			fmt.Fprintf(os.Stderr, "[ansible] inventory (delete):\n%s\n", safeInv)
		}
	} else {
		cmd.Stderr = &stderr
	}

	// Best-effort delete (ignore error; we will clear DB regardless to unblock user)
	_ = cmd.Run()

	// Clear reservation and free the machine
	tx := h.db.MustBegin()
	tx.MustExec("DELETE FROM reservations WHERE id=?", id)
	tx.MustExec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id=?", x.MID)
	_ = tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
