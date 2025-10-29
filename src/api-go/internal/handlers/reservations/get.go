package reservations

import (
	"net/http"
	"time"

	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
	"github.com/gin-gonic/gin"
)

// Get returns a single reservation by id.
// - Admins can fetch any reservation.
// - Non-admins can only fetch their own reservation.
// The response is minimal for non-admins to keep it simple and safe.
func (h *Handler) Get(c *gin.Context) {
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

	// Non-admins can only fetch their own reservation
	if !isAdmin && x.Username != user {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	// Compute seconds remaining (0 if expired or unset)
	sec := 0
	if x.ReservedUntil != nil {
		if d := int(x.ReservedUntil.Sub(time.Now().UTC()).Seconds()); d > 0 {
			sec = d
		}
	}

	// Keep response shaping clear and small
	if isAdmin {
		c.JSON(http.StatusOK, gin.H{
			"id":                x.ID,
			"user_id":           x.UserID,
			"username":          x.Username,
			"machine":           x.Machine,
			"host":              x.Host,
			"port":              x.Port,
			"reserved_until":    common.FormatTimePtr(x.ReservedUntil),
			"seconds_remaining": sec,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"machine":           x.Machine,
		"host":              x.Host,
		"port":              x.Port,
		"reserved_until":    common.FormatTimePtr(x.ReservedUntil),
		"seconds_remaining": sec,
	})
}
