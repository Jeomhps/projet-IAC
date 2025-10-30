package machines

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// Update modifies a machine's fields.
// Allowed fields: host, port, user, password, enabled, online, name, spare_pool.
// Admin-only (enforced by router middleware).
func (h *Handler) Update(c *gin.Context) {
	origName := c.Param("name")

	// Accept a generic map for partial updates (KISS)
	var in map[string]any
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request"})
		return
	}

	fields := []string{}
	args := []any{}

	// Whitelist of updatable fields
	for _, f := range []string{"host", "port", "user", "password", "enabled", "online", "name", "spare_pool"} {
		v, ok := in[f]
		if !ok {
			continue
		}

		// Port should be numeric if provided
		if f == "port" {
			switch vv := v.(type) {
			case float64:
				args = append(args, int(vv))
			case string:
				n, err := strconv.Atoi(vv)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "port must be a number"})
					return
				}
				args = append(args, n)
			default:
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "port must be a number"})
				return
			}
		} else {
			args = append(args, v)
		}
		fields = append(fields, f+"=?")
	}

	if len(fields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_request", "message": "no fields"})
		return
	}

	// Apply update: WHERE name matches the original path param
	args = append(args, origName)
	if _, err := h.db.Exec("UPDATE machines SET "+strings.Join(fields, ",")+" WHERE name=?", args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server_error"})
		return
	}

	// Delegate response to Get handler for consistency.
	// Note: if name was changed, this will look up the original name and may return not_found.
	// This mirrors previous behavior and keeps the flow simple.
	h.Get(c)
}
