package machines

import (
	"net/http"
	"strings"

	"github.com/Jeomhps/projet-IAC/api-go/internal/db"
	"github.com/Jeomhps/projet-IAC/api-go/internal/handlers/common"
	"github.com/gin-gonic/gin"
)

// List returns machines with optional filters.
// - Query params:
//   - eligible=true|false -> filter by (enabled=1 AND online=1 AND reserved=0) or its negation
//   - reserved=true|false -> filter by reserved flag
//   - name=<prefix>       -> filter by name prefix (LIKE prefix%)
//
// - Non-admins only see enabled machines.
func (h *Handler) List(c *gin.Context) {
	q := "SELECT * FROM machines"
	args := []any{}
	where := []string{}

	// Filter by eligible pool if provided
	if v := c.Query("eligible"); v != "" {
		if strings.EqualFold(v, "true") || v == "1" {
			where = append(where, "(enabled=1 AND online=1 AND reserved=0)")
		} else {
			where = append(where, "NOT (enabled=1 AND online=1 AND reserved=0)")
		}
	}

	// Filter by reserved flag if provided
	if v := c.Query("reserved"); v != "" {
		if strings.EqualFold(v, "true") || v == "1" {
			where = append(where, "reserved=1")
		} else {
			where = append(where, "reserved=0")
		}
	}

	// Filter by name prefix if provided
	if v := c.Query("name"); v != "" {
		where = append(where, "name LIKE ?")
		args = append(args, v+"%")
	}

	// Non-admin users only see enabled machines
	if !c.GetBool("is_admin") {
		where = append(where, "enabled=1")
	}

	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY name ASC"

	var ms []db.Machine
	if err := h.db.Select(&ms, q, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	admin := c.GetBool("is_admin")
	out := make([]gin.H, 0, len(ms))
	for _, m := range ms {
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
		}
		out = append(out, item)
	}

	c.JSON(http.StatusOK, out)
}
