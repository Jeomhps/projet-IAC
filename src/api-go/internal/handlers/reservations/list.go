package reservations

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
	"github.com/gin-gonic/gin"
)

// Handler exposes reservation-related endpoints in a small, focused type.
// Keep it simple: hold only what's needed for the specific handlers in this file.
type Handler struct {
	db *db.DB
}

// NewHandler wires a DB into the reservations handler.
func NewHandler(d *db.DB) *Handler {
	return &Handler{db: d}
}

// List returns reservations for the current user.
// - Admins see all reservations.
// - Non-admins see only their reservations AND only those tied to enabled machines.
// Response is intentionally minimal for non-admins.
func (h *Handler) List(c *gin.Context) {
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

	// Build a simple query with minimal branching: admins see all; users see theirs + enabled machines only.
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

	// Keep response shaping clear and small; only include admin-only fields when needed.
	for _, x := range rows {
		sec := 0
		if x.ReservedUntil != nil {
			if d := int(x.ReservedUntil.Sub(now).Seconds()); d > 0 {
				sec = d
			}
		}
		if isAdmin {
			out = append(out, gin.H{
				"id":                x.ID,
				"user_id":           x.UserID,
				"username":          x.Username,
				"machine":           x.Machine,
				"host":              x.Host,
				"port":              x.Port,
				"reserved_until":    common.FormatTimePtr(x.ReservedUntil),
				"seconds_remaining": sec,
			})
		} else {
			out = append(out, gin.H{
				"machine":           x.Machine,
				"host":              x.Host,
				"port":              x.Port,
				"reserved_until":    common.FormatTimePtr(x.ReservedUntil),
				"seconds_remaining": sec,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"reservations": out})
}
