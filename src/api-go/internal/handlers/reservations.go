package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/gin-gonic/gin"
)

type Reservations struct{
	db *db.DB
	playbook string
}
func NewReservations(d *db.DB, playbookPath string) *Reservations { return &Reservations{db:d, playbook:playbookPath} }

func (h *Reservations) List(c *gin.Context) {
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	type row struct {
		ID int `db:"id"`
		UserID int `db:"user_id"`
		Username string `db:"username"`
		ReservedUntil *time.Time `db:"reserved_until"`
		Machine string `db:"name"`
		Host string `db:"host"`
		Port int `db:"port"`
	}
	q := `SELECT r.id, r.user_id, r.username, r.reserved_until, m.name, m.host, m.port
	      FROM reservations r JOIN machines m ON m.id=r.machine_id`
	args := []any{}
	if !isAdmin {
		q += " WHERE r.username=?"
		args = append(args, user)
	}
	q += " ORDER BY r.id DESC"

	var rows []row
	if err := h.db.Select(&rows, q, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error"}); return
	}

	now := time.Now().UTC()
	out := make([]gin.H, 0, len(rows))
	for _, x := range rows {
		sec := 0
		if x.ReservedUntil != nil {
			if d := int(x.ReservedUntil.Sub(now).Seconds()); d > 0 { sec = d }
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
		ID int `db:"id"`
		UserID int `db:"user_id"`
		Username string `db:"username"`
		ReservedUntil *time.Time `db:"reserved_until"`
		Machine string `db:"name"`
		Host string `db:"host"`
		Port int `db:"port"`
	}
	var x row
	if err := h.db.Get(&x, `SELECT r.id,r.user_id,r.username,r.reserved_until,m.name,m.host,m.port
		FROM reservations r JOIN machines m ON m.id=r.machine_id WHERE r.id=?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error":"not_found"}); return
	}
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error":"forbidden"}); return
	}
	sec := 0
	if x.ReservedUntil != nil {
		if d := int(x.ReservedUntil.Sub(time.Now().UTC()).Seconds()); d > 0 { sec = d }
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
		Count int `json:"count"`
		Duration int `json:"duration_minutes"`
		Password string `json:"reservation_password"`
		Username string `json:"username"` // rejected
	}
	if err := c.ShouldBindJSON(&in); err != nil || in.Count <= 0 || in.Duration <= 0 || in.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request"}); return
	}
	if strings.TrimSpace(in.Username) != "" {
		c.JSON(http.StatusBadRequest, gin.H{"error":"invalid_request","message":"username must not be provided"}); return
	}
	// Lookup user_id
	var uid int
	if err := h.db.Get(&uid, "SELECT id FROM users WHERE username=?", user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error"}); return
	}
	// Eligible machines
	var ms []db.Machine
	if err := h.db.Select(&ms, "SELECT * FROM machines WHERE enabled=1 AND online=1 AND reserved=0 ORDER BY id ASC"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error"}); return
	}
	if len(ms) < in.Count {
		c.JSON(http.StatusConflict, gin.H{"error":"not_enough_available","available": len(ms)}); return
	}
	reserved := ms[:in.Count]
	until := time.Now().UTC().Add(time.Duration(in.Duration) * time.Minute)

	// Inventory
	tmp, _ := os.CreateTemp("", "inv-*.ini")
	defer os.Remove(tmp.Name())
	for _, m := range reserved {
		tmp.WriteString(m.Name + " ansible_host=" + m.Host + " ansible_port=" + strconv.Itoa(m.Port) +
			" ansible_user=" + m.User + " ansible_password=" + m.Password + "\n")
	}
	tmp.Close()

	// Hash password with openssl SHA-512-crypt ($6$)
	hashed, err := hashSHA512Crypt(in.Password)
	if err != nil || strings.TrimSpace(hashed) == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error","message":"hashing failed"}); return
	}

	// Run playbook
	cmd := exec.Command("ansible-playbook", "-i", tmp.Name(), h.playbook, "--extra-vars",
		fmt.Sprintf("username=%s hashed_password=%s user_action=create", user, hashed))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"ansible_failed","details": stderr.String()}); return
	}

	// Persist
	tx := h.db.MustBegin()
	for _, m := range reserved {
		tx.MustExec("INSERT INTO reservations (machine_id,user_id,username,reserved_until) VALUES (?,?,?,?)", m.ID, uid, user, until)
		tx.MustExec("UPDATE machines SET reserved=1,reserved_by=?,reserved_until=? WHERE id=?", user, until, m.ID)
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error":"server_error"}); return
	}
	out := make([]gin.H, 0, len(reserved))
	for _, m := range reserved {
		out = append(out, gin.H{"machine": m.Name, "host": m.Host, "port": m.Port})
	}
	c.JSON(http.StatusCreated, gin.H{
		"machines": out,
		"reserved_until": until.UTC().Format(time.RFC3339),
		"duration_minutes": in.Duration,
	})
}

func (h *Reservations) Delete(c *gin.Context) {
	id := c.Param("id")
	user := c.GetString("user")
	isAdmin := c.GetBool("is_admin")

	// Load reservation and machine
	type row struct {
		ID int `db:"id"`
		Username string `db:"username"`
		MID int `db:"machine_id"`
		MName string `db:"name"`
		Host string `db:"host"`
		Port int `db:"port"`
		User string `db:"user"`
		Pass string `db:"password"`
	}
	var x row
	if err := h.db.Get(&x, `SELECT r.id,r.username,m.id as machine_id,m.name,m.host,m.port,m.user,m.password
		FROM reservations r JOIN machines m ON m.id=r.machine_id WHERE r.id=?`, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error":"not_found"}); return
	}
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error":"forbidden"}); return
	}
	tmp, _ := os.CreateTemp("", "inv-*.ini")
	defer os.Remove(tmp.Name())
	tmp.WriteString(fmt.Sprintf("%s ansible_host=%s ansible_port=%d ansible_user=%s ansible_password=%s\n", x.MName, x.Host, x.Port, x.User, x.Pass))
	tmp.Close()

	cmd := exec.Command("ansible-playbook", "-i", tmp.Name(), h.playbook, "--extra-vars", fmt.Sprintf("username=%s user_action=delete", x.Username))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // best-effort

	// Clear DB
	tx := h.db.MustBegin()
	tx.MustExec("DELETE FROM reservations WHERE id=?", id)
	tx.MustExec("UPDATE machines SET reserved=0,reserved_by=NULL,reserved_until=NULL WHERE id=?", x.MID)
	_ = tx.Commit()

	c.JSON(http.StatusOK, gin.H{"message":"deleted"})
}

// hashSHA512Crypt uses openssl to generate $6$-style SHA-512-crypt hashes.
func hashSHA512Crypt(password string) (string, error) {
	cmd := exec.Command("openssl", "passwd", "-6", password)
	var out bytes.Buffer; var errb bytes.Buffer
	cmd.Stdout = &out; cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("openssl: %v: %s", err, errb.String())
	}
	return strings.TrimSpace(out.String()), nil
}
