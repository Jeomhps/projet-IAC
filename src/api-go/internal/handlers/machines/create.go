package machines

import (
	"net/http"
	"strings"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
	"github.com/gin-gonic/gin"
)

// Create registers a new machine.
// Admin-only (enforced by router middleware).
func (h *Handler) Create(c *gin.Context) {
	// Minimal request validation (KISS)
	var in struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Port     int    `json:"port"`
		User     string `json:"user"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&in); err != nil ||
		strings.TrimSpace(in.Name) == "" ||
		strings.TrimSpace(in.Host) == "" ||
		strings.TrimSpace(in.User) == "" ||
		strings.TrimSpace(in.Password) == "" ||
		in.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	// Insert; defaults: reserved=0, enabled=1, online=1
	if _, err := h.db.Exec(
		`INSERT INTO machines (name,host,port,user,password,reserved,reserved_by,reserved_until,enabled,online)
		 VALUES (?,?,?,?,?,0,NULL,NULL,1,1)`,
		in.Name, in.Host, in.Port, in.User, in.Password,
	); err != nil {
		// Most likely a duplicate name
		c.JSON(http.StatusConflict, gin.H{"error": "conflict", "message": "Machine exists"})
		return
	}

	// Load the newly created row for response shaping
	var m db.Machine
	_ = h.db.Get(&m, "SELECT * FROM machines WHERE name=?", in.Name)

	// Admin route: return full details
	c.JSON(http.StatusCreated, gin.H{
		"name":               m.Name,
		"host":               m.Host,
		"port":               m.Port,
		"user":               m.User,
		"reserved":           m.Reserved,
		"reserved_by":        m.ReservedBy,
		"reserved_until":     common.FormatTimePtr(m.ReservedUntil),
		"enabled":            m.Enabled,
		"online":             m.Online,
		"last_seen_at":       common.FormatTimePtr(m.LastSeenAt),
		"reserve_fail_count": m.ReserveFailCount,
		"quarantine_until":   common.FormatTimePtr(m.QuarantineUntil),
	})
}
