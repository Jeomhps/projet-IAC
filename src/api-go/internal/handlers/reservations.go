package handlers

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/runner"
	"github.com/gin-gonic/gin"
)

type Reservations struct {
	db       *db.DB
	playbook string
	runner   runner.PlaybookRunner
}

func NewReservations(d *db.DB, playbookPath string, pr runner.PlaybookRunner) *Reservations {
	return &Reservations{db: d, playbook: playbookPath, runner: pr}
}

func (h *Reservations) List(c *gin.Context) {
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	type row struct {
		ID            int        `db:"id"`
		UserID        int        `db:"user_id"`
		Username      string     `db:"username"`
		ReservedUntil *time.Time `db:"reserved_until"`
		Machine       string     `db:"name"`
		Host          string     `db:"host"`
		Port          int        `db:"port"`
	}
	q := `SELECT r.id, r.user_id, r.username, r.reserved_until, m.name, m.host, m.port
	      FROM reservations r JOIN machines m ON m.id=r.machine_id`
	args := []any{}
	if !isAdmin {
		q += " WHERE r.username=? AND m.enabled=1"
		args = append(args, user)
	}
	q += " ORDER BY r.id DESC"

	var rows []row
	if err := h.db.Select(&rows, q, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	now := time.Now().UTC()
	out := make([]gin.H, 0, len(rows))
	for _, x := range rows {
		sec := 0
		if x.ReservedUntil != nil {
			if d := int(x.ReservedUntil.Sub(now).Seconds()); d > 0 {
				sec = d
			}
		}
		if isAdmin {
			out = append(out, gin.H{
				"id": x.ID, "user_id": x.UserID, "username": x.Username,
				"machine": x.Machine, "host": x.Host, "port": x.Port,
				"reserved_until": formatTimePtr(x.ReservedUntil), "seconds_remaining": sec,
			})
		} else {
			out = append(out, gin.H{
				"machine": x.Machine, "host": x.Host, "port": x.Port,
				"reserved_until": formatTimePtr(x.ReservedUntil), "seconds_remaining": sec,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"reservations": out})
}

func (h *Reservations) Get(c *gin.Context) {
	id := c.Param("id")
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	type row struct {
		ID            int        `db:"id"`
		UserID        int        `db:"user_id"`
		Username      string     `db:"username"`
		ReservedUntil *time.Time `db:"reserved_until"`
		Machine       string     `db:"name"`
		Host          string     `db:"host"`
		Port          int        `db:"port"`
	}
	var x row
	if err := h.db.Get(&x, `SELECT r.id,r.user_id,r.username,r.reserved_until,m.name,m.host,m.port
		FROM reservations r JOIN machines m ON m.id=r.machine_id WHERE r.id=?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	sec := 0
	if x.ReservedUntil != nil {
		if d := int(x.ReservedUntil.Sub(time.Now().UTC()).Seconds()); d > 0 {
			sec = d
		}
	}
	if isAdmin {
		c.JSON(http.StatusOK, gin.H{
			"id": x.ID, "user_id": x.UserID, "username": x.Username,
			"machine": x.Machine, "host": x.Host, "port": x.Port,
			"reserved_until": formatTimePtr(x.ReservedUntil), "seconds_remaining": sec,
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"machine": x.Machine, "host": x.Host, "port": x.Port,
		"reserved_until": formatTimePtr(x.ReservedUntil), "seconds_remaining": sec,
	})
}

func (h *Reservations) Create(c *gin.Context) {
	user := c.GetString("user")
	var in struct {
		Count    int    `json:"count"`
		Duration int    `json:"duration_minutes"`
		Password string `json:"reservation_password"`
		Username string `json:"username"` // rejected
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Count <= 0 || in.Duration <= 0 || in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}
	if strings.TrimSpace(in.Username) != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "username must not be provided"})
		return
	}
	// Lookup user_id
	var uid int
	if err := h.db.Get(&uid, "SELECT id FROM users WHERE username=?", user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}
	// Reserve-first (transactional) to prevent collisions
	tx := h.db.MustBegin()
	var ids []int
	// Lock eligible rows and pick N
	if err := tx.Select(&ids, "SELECT id FROM machines WHERE enabled=1 AND online=1 AND reserved=0 ORDER BY id ASC LIMIT ? FOR UPDATE", in.Count); err != nil {
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

	// Mark them reserved inside the TX
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

	// Load the reserved machines to build inventory
	var reserved []db.Machine
	selArgs := make([]any, 0, len(ids))
	for i := range ids {
		selArgs = append(selArgs, ids[i])
	}
	query := "SELECT * FROM machines WHERE id IN (" + strings.Join(ph, ",") + ") ORDER BY id ASC"
	if err := h.db.Select(&reserved, query, selArgs...); err != nil {
		// revert reserved flags if we failed to load rows
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Inventory
	tmp, _ := os.CreateTemp("", "inv-*.ini")
	defer os.Remove(tmp.Name())
	for _, m := range reserved {
		tmp.WriteString(m.Name + " ansible_host=" + m.Host + " ansible_port=" + strconv.Itoa(m.Port) +
			" ansible_user=" + m.User + " ansible_password=" + safeInvVal(m.Password) + "\n")
	}
	tmp.Close()

	// Hash password with openssl SHA-512-crypt ($6$)
	hashed, err := hashSHA512Crypt(in.Password)
	if err != nil || strings.TrimSpace(hashed) == "" {
		// revert reserved flags
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error", "message": "hashing failed"})
		return
	}

	level := logLevel()
	args := []string{"ansible-playbook"}
	if vflags := ansibleVerbosityFlags(level); len(vflags) > 0 {
		args = append(args, vflags...)
	}
	args = append(args,
		"-f", strconv.Itoa(h.runner.Forks),
		"-i", tmp.Name(),
		h.playbook,
		"--extra-vars", fmt.Sprintf("username=%s hashed_password=%s user_action=create", user, hashed),
	)
	cmd := exec.Command(args[0], args[1:]...)
	var stderr bytes.Buffer

	// Stream logs and sanitize inventory in debug/trace modes
	if streamAnsible(level) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderr)
		if inv, _ := os.ReadFile(tmp.Name()); len(inv) > 0 {
			log.Printf("[ansible] inventory (create):\n%s", sanitizeInventory(string(inv)))
		}
	} else {
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		// revert reserved flags on failure
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ansible_failed", "details": strings.TrimSpace(stderr.String())})
		return
	}

	// Persist reservations (machines already marked reserved)
	txi := h.db.MustBegin()
	for _, m := range reserved {
		txi.MustExec("INSERT INTO reservations (machine_id,user_id,username,reserved_until) VALUES (?,?,?,?)", m.ID, uid, user, until)
	}
	if err := txi.Commit(); err != nil {
		// rollback reservation flags if insertion fails
		txr := h.db.MustBegin()
		_, _ = txr.Exec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id IN ("+strings.Join(ph, ",")+")", selArgs...)
		_ = txr.Commit()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

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

func (h *Reservations) Delete(c *gin.Context) {
	id := c.Param("id")
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	// Load reservation and machine
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
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	// Best-effort delete with standardized runner; logs streamed via runner
	_ = h.runner.RunDeleteUserSingleHost(c.Request.Context(), x.MName, x.Host, x.Port, x.User, x.Pass, x.Username)

	// Clear DB
	tx := h.db.MustBegin()
	tx.MustExec("DELETE FROM reservations WHERE id=?", id)
	tx.MustExec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id=?", x.MID)
	_ = tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func safeInvVal(s string) string {
	// escape spaces, equals, and backslashes for INI-like inventory
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ` `, `\ `)
	s = strings.ReplaceAll(s, `=`, `\=`)
	return s
}

// ---- Logging helpers ----

func logLevel() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if v == "" {
		return "info"
	}
	return v
}

func streamAnsible(level string) bool {
	// Stream stdout/stderr for debug and all trace levels
	return level == "debug" || strings.HasPrefix(level, "trace")
}

func ansibleVerbosityFlags(level string) []string {
	switch level {
	case "trace3", "trace-3":
		return []string{"-vvv"}
	case "trace2", "trace-2":
		return []string{"-vv"}
	case "trace", "trace1", "trace-1":
		return []string{"-v"}
	default: // info, debug
		return nil
	}
}

var rePass = regexp.MustCompile(`(?m)(ansible_password=)(\S+)`)

func sanitizeInventory(s string) string {
	// redact ansible_password values in logged inventory
	return rePass.ReplaceAllString(s, "${1}***")
}

// ---- Crypto helper (unchanged) ----

// hashSHA512Crypt uses openssl to generate $6$-style SHA-512-crypt hashes.
func hashSHA512Crypt(password string) (string, error) {
	cmd := exec.Command("openssl", "passwd", "-6", password)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("openssl: %v: %s", err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}
