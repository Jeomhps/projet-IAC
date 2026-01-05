package machines

import (
	"net/http"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
	"github.com/gin-gonic/gin"
)

// Get returns a single machine by name.
// - Non-admins are not allowed to view disabled machines.
func (h *Handler) Get(c *gin.Context) {
	name := c.Param("name")

	var m db.Machine
	if err := h.db.Get(&m, "SELECT * FROM machines WHERE name=?", name); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	// Non-admins cannot view disabled or spare-pool machines
	if !c.GetBool("is_admin") && (!m.Enabled || m.SparePool) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
		return
	}

	admin := c.GetBool("is_admin")
	item := gin.H{
		"name":           m.Name,
		"reserved":       m.Reserved,
		"reserved_until": common.FormatTimePtr(m.ReservedUntil),
	}

	if admin {
		item["host"] = m.Host
		item["port"] = m.Port
		item["user"] = m.User
		item["reserved_by"] = m.ReservedBy
		item["enabled"] = m.Enabled
		item["online"] = m.Online
		item["last_seen_at"] = common.FormatTimePtr(m.LastSeenAt)
		item["reserve_fail_count"] = m.ReserveFailCount
		item["quarantine_until"] = common.FormatTimePtr(m.QuarantineUntil)
		item["spare_pool"] = m.SparePool
	}

	c.JSON(http.StatusOK, item)
}
